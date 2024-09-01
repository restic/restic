package filter

import (
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
}

func (opts *IncludePatternOptions) Add(f *pflag.FlagSet) {
	f.StringArrayVarP(&opts.Includes, "include", "i", nil, "include a `pattern` (can be specified multiple times)")
	f.StringArrayVar(&opts.InsensitiveIncludes, "iinclude", nil, "same as --include `pattern` but ignores the casing of filenames")
	f.StringArrayVar(&opts.IncludeFiles, "include-file", nil, "read include patterns from a `file` (can be specified multiple times)")
	f.StringArrayVar(&opts.InsensitiveIncludeFiles, "iinclude-file", nil, "same as --include-file but ignores casing of `file`names in patterns")
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
	return fs, nil
}

// IncludeByPattern returns a IncludeByNameFunc which includes files that match
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

// IncludeByInsensitivePattern returns a IncludeByNameFunc which includes files that match
// one of the patterns, ignoring the casing of the filenames.
func IncludeByInsensitivePattern(patterns []string, warnf func(msg string, args ...interface{})) IncludeByNameFunc {
	for index, path := range patterns {
		patterns[index] = strings.ToLower(path)
	}

	includeFunc := IncludeByPattern(patterns, warnf)
	return func(item string) (matched bool, childMayMatch bool) {
		return includeFunc(strings.ToLower(item))
	}
}
