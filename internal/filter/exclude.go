package filter

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"strings"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/textfile"
	"github.com/spf13/pflag"
)

// RejectByNameFunc is a function that takes a filename of a
// file that would be included in the backup. The function returns true if it
// should be excluded (rejected) from the backup.
type RejectByNameFunc func(path string) bool

// RejectByPattern returns a RejectByNameFunc which rejects files that match
// one of the patterns.
func RejectByPattern(patterns []string, warnf func(msg string, args ...interface{})) RejectByNameFunc {
	parsedPatterns := ParsePatterns(patterns)
	return func(item string) bool {
		matched, err := List(parsedPatterns, item)
		if err != nil {
			warnf("error for exclude pattern: %v", err)
		}

		if matched {
			debug.Log("path %q excluded by an exclude pattern", item)
			return true
		}

		return false
	}
}

// RejectByInsensitivePattern is like RejectByPattern but case insensitive.
func RejectByInsensitivePattern(patterns []string, warnf func(msg string, args ...interface{})) RejectByNameFunc {
	for index, path := range patterns {
		patterns[index] = strings.ToLower(path)
	}

	rejFunc := RejectByPattern(patterns, warnf)
	return func(item string) bool {
		return rejFunc(strings.ToLower(item))
	}
}

// readPatternsFromFiles reads all files and returns the list of
// patterns. For each line, leading and trailing white space is removed
// and comment lines are ignored. For each remaining pattern, environment
// variables are resolved. For adding a literal dollar sign ($), write $$ to
// the file.
func readPatternsFromFiles(files []string) ([]string, error) {
	getenvOrDollar := func(s string) string {
		if s == "$" {
			return "$"
		}
		return os.Getenv(s)
	}

	var patterns []string
	for _, filename := range files {
		err := func() (err error) {
			data, err := textfile.Read(filename)
			if err != nil {
				return err
			}

			scanner := bufio.NewScanner(bytes.NewReader(data))
			for scanner.Scan() {
				line := strings.TrimSpace(scanner.Text())

				// ignore empty lines
				if line == "" {
					continue
				}

				// strip comments
				if strings.HasPrefix(line, "#") {
					continue
				}

				line = os.Expand(line, getenvOrDollar)
				patterns = append(patterns, line)
			}
			return scanner.Err()
		}()
		if err != nil {
			return nil, fmt.Errorf("failed to read patterns from file %q: %w", filename, err)
		}
	}
	return patterns, nil
}

type ExcludePatternOptions struct {
	Excludes                []string
	InsensitiveExcludes     []string
	ExcludeFiles            []string
	InsensitiveExcludeFiles []string
}

func (opts *ExcludePatternOptions) Add(f *pflag.FlagSet) {
	f.StringArrayVarP(&opts.Excludes, "exclude", "e", nil, "exclude a `pattern` (can be specified multiple times)")
	f.StringArrayVar(&opts.InsensitiveExcludes, "iexclude", nil, "same as --exclude `pattern` but ignores the casing of filenames")
	f.StringArrayVar(&opts.ExcludeFiles, "exclude-file", nil, "read exclude patterns from a `file` (can be specified multiple times)")
	f.StringArrayVar(&opts.InsensitiveExcludeFiles, "iexclude-file", nil, "same as --exclude-file but ignores casing of `file`names in patterns")
}

func (opts *ExcludePatternOptions) Empty() bool {
	return len(opts.Excludes) == 0 && len(opts.InsensitiveExcludes) == 0 && len(opts.ExcludeFiles) == 0 && len(opts.InsensitiveExcludeFiles) == 0
}

func (opts ExcludePatternOptions) CollectPatterns(warnf func(msg string, args ...interface{})) ([]RejectByNameFunc, error) {
	var fs []RejectByNameFunc
	// add patterns from file
	if len(opts.ExcludeFiles) > 0 {
		excludePatterns, err := readPatternsFromFiles(opts.ExcludeFiles)
		if err != nil {
			return nil, err
		}

		if err := ValidatePatterns(excludePatterns); err != nil {
			return nil, errors.Fatalf("--exclude-file: %s", err)
		}

		opts.Excludes = append(opts.Excludes, excludePatterns...)
	}

	if len(opts.InsensitiveExcludeFiles) > 0 {
		excludes, err := readPatternsFromFiles(opts.InsensitiveExcludeFiles)
		if err != nil {
			return nil, err
		}

		if err := ValidatePatterns(excludes); err != nil {
			return nil, errors.Fatalf("--iexclude-file: %s", err)
		}

		opts.InsensitiveExcludes = append(opts.InsensitiveExcludes, excludes...)
	}

	if len(opts.InsensitiveExcludes) > 0 {
		if err := ValidatePatterns(opts.InsensitiveExcludes); err != nil {
			return nil, errors.Fatalf("--iexclude: %s", err)
		}

		fs = append(fs, RejectByInsensitivePattern(opts.InsensitiveExcludes, warnf))
	}

	if len(opts.Excludes) > 0 {
		if err := ValidatePatterns(opts.Excludes); err != nil {
			return nil, errors.Fatalf("--exclude: %s", err)
		}

		fs = append(fs, RejectByPattern(opts.Excludes, warnf))
	}
	return fs, nil
}
