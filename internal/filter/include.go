package filter

import (
	"bufio"
	"io"
	"os"
	"strings"

	"github.com/restic/restic/internal/errors"
	"github.com/spf13/pflag"
)

// IncludeByNameFunc is a function that takes a filename that should be included
// in the restore process and returns whether it should be included.
type IncludeByNameFunc func(item string) (matched bool, childMayMatch bool)

type IncludePatternOptions struct {
	Includes                []string
	InsensitiveIncludes     []string
	IncludeFiles            []string
	InsensitiveIncludeFiles []string
	IncludeFilesRaw         []string
}

func (opts *IncludePatternOptions) Add(f *pflag.FlagSet) {
	f.StringArrayVarP(&opts.Includes, "include", "i", nil, "include a `pattern` (can be specified multiple times)")
	f.StringArrayVar(&opts.InsensitiveIncludes, "iinclude", nil, "same as --include `pattern` but ignores the casing of filenames")
	f.StringArrayVar(&opts.IncludeFiles, "include-file", nil, "read include patterns from a `file` (can be specified multiple times)")
	f.StringArrayVar(&opts.InsensitiveIncludeFiles, "iinclude-file", nil, "same as --include-file but ignores casing of `file`names in patterns")
	f.StringArrayVar(&opts.IncludeFilesRaw, "include-from-raw", nil, "read literal NUL-separated include paths from `file` (can be specified multiple times); paths are matched verbatim, glob metacharacters lose their meaning")
}

func (opts *IncludePatternOptions) Empty() bool {
	return len(opts.Includes) == 0 && len(opts.InsensitiveIncludes) == 0 && len(opts.IncludeFiles) == 0 && len(opts.InsensitiveIncludeFiles) == 0 && len(opts.IncludeFilesRaw) == 0
}

func (opts IncludePatternOptions) CollectPatterns(warnf func(msg string, args ...interface{})) ([]IncludeByNameFunc, error) {
	var fs []IncludeByNameFunc
	if len(opts.IncludeFiles) > 0 {
		includePatterns, err := readPatternsFromFiles(opts.IncludeFiles)
		if err != nil {
			return nil, err
		}

		if err := ValidatePatterns(includePatterns); err != nil {
			return nil, errors.Fatalf("--include-file: %s", err)
		}

		opts.Includes = append(opts.Includes, includePatterns...)
	}

	if len(opts.InsensitiveIncludeFiles) > 0 {
		includePatterns, err := readPatternsFromFiles(opts.InsensitiveIncludeFiles)
		if err != nil {
			return nil, err
		}

		if err := ValidatePatterns(includePatterns); err != nil {
			return nil, errors.Fatalf("--iinclude-file: %s", err)
		}

		opts.InsensitiveIncludes = append(opts.InsensitiveIncludes, includePatterns...)
	}

	if len(opts.InsensitiveIncludes) > 0 {
		if err := ValidatePatterns(opts.InsensitiveIncludes); err != nil {
			return nil, errors.Fatalf("--iinclude: %s", err)
		}

		fs = append(fs, IncludeByInsensitivePattern(opts.InsensitiveIncludes, warnf))
	}

	if len(opts.Includes) > 0 {
		if err := ValidatePatterns(opts.Includes); err != nil {
			return nil, errors.Fatalf("--include: %s", err)
		}

		fs = append(fs, IncludeByPattern(opts.Includes, warnf))
	}

	if len(opts.IncludeFilesRaw) > 0 {
		rawPatterns, err := readPathsFromFilesRaw(opts.IncludeFilesRaw)
		if err != nil {
			return nil, err
		}
		// An empty listing adds no constraint, just like an empty
		// --include-file. Only register a matcher when there is at least
		// one path; otherwise the restore would match nothing rather than
		// everything. Literal patterns need no validation: there is no glob
		// to be malformed and the empty entry is already rejected while
		// reading the file.
		if len(rawPatterns) > 0 {
			fs = append(fs, IncludeByLiteralPattern(rawPatterns))
		}
	}
	return fs, nil
}

// IncludeByLiteralPattern returns an IncludeByNameFunc that matches items
// whose path is exactly one of the supplied literal patterns (or whose path
// is a prefix that might still lead to one). Unlike IncludeByPattern this
// matcher never invokes the glob engine, so patterns containing '[', ']',
// '*' or '?' are matched verbatim.
func IncludeByLiteralPattern(patterns []string) IncludeByNameFunc {
	parsed := ParseLiteralPatterns(patterns)
	return func(item string) (matched bool, childMayMatch bool) {
		matched, childMayMatch, _ = ListWithChild(parsed, item)
		return matched, childMayMatch
	}
}

// readPathsFromFilesRaw reads NUL-separated paths from each of the named
// files (or stdin if a name is "-") and returns the combined list. Mirrors
// readFilenamesFromFileRaw used by --files-from-raw on the backup side.
func readPathsFromFilesRaw(files []string) ([]string, error) {
	var out []string
	for _, name := range files {
		paths, err := readOneRawPathFile(name)
		if err != nil {
			return nil, errors.Fatalf("failed to read paths from file %q: %s", name, err)
		}
		out = append(out, paths...)
	}
	return out, nil
}

func readOneRawPathFile(filename string) (paths []string, err error) {
	var r io.ReadCloser
	if filename == "-" {
		r = os.Stdin
	} else {
		r, err = os.Open(filename)
		if err != nil {
			return nil, err
		}
		defer func() {
			if cerr := r.Close(); err == nil {
				err = cerr
			}
		}()
	}

	br := bufio.NewReader(r)
	for {
		name, readErr := br.ReadString(0)
		switch readErr {
		case nil:
		case io.EOF:
			if name == "" {
				return paths, nil
			}
			return nil, errors.Fatal("trailing zero byte missing")
		default:
			return nil, readErr
		}
		name = name[:len(name)-1]
		if name == "" {
			return nil, errors.Fatal("empty path in listing")
		}
		paths = append(paths, name)
	}
}

// IncludeByPattern returns an IncludeByNameFunc which includes files that match
// one of the patterns.
func IncludeByPattern(patterns []string, warnf func(msg string, args ...interface{})) IncludeByNameFunc {
	parsedPatterns := ParsePatterns(patterns)
	return func(item string) (matched bool, childMayMatch bool) {
		matched, childMayMatch, err := ListWithChild(parsedPatterns, item)
		if err != nil {
			warnf("error for include pattern: %v", err)
		}

		return matched, childMayMatch
	}
}

// IncludeByInsensitivePattern returns an IncludeByNameFunc which includes files that match
// one of the patterns, ignoring the casing of the filenames.
func IncludeByInsensitivePattern(patterns []string, warnf func(msg string, args ...interface{})) IncludeByNameFunc {
	lowerPatterns := make([]string, len(patterns))
	for index, path := range patterns {
		lowerPatterns[index] = strings.ToLower(path)
	}

	includeFunc := IncludeByPattern(lowerPatterns, warnf)
	return func(item string) (matched bool, childMayMatch bool) {
		return includeFunc(strings.ToLower(item))
	}
}
