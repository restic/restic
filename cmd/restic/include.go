package main

import (
	"strings"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/filter"
	"github.com/spf13/pflag"
)

// IncludeByNameFunc is a function that takes a filename that should be included
// in the restore process and returns whether it should be included.
type IncludeByNameFunc func(item string) (matched bool, childMayMatch bool)

type includePatternOptions struct {
	Includes                []string
	InsensitiveIncludes     []string
	IncludeFiles            []string
	InsensitiveIncludeFiles []string
}

func initIncludePatternOptions(f *pflag.FlagSet, opts *includePatternOptions) {
	f.StringArrayVarP(&opts.Includes, "include", "i", nil, "include a `pattern` (can be specified multiple times)")
	f.StringArrayVar(&opts.InsensitiveIncludes, "iinclude", nil, "same as --include `pattern` but ignores the casing of filenames")
	f.StringArrayVar(&opts.IncludeFiles, "include-file", nil, "read include patterns from a `file` (can be specified multiple times)")
	f.StringArrayVar(&opts.InsensitiveIncludeFiles, "iinclude-file", nil, "same as --include-file but ignores casing of `file`names in patterns")
}

func (opts includePatternOptions) CollectPatterns() ([]IncludeByNameFunc, error) {
	var fs []IncludeByNameFunc
	if len(opts.IncludeFiles) > 0 {
		includePatterns, err := readPatternsFromFiles(opts.IncludeFiles)
		if err != nil {
			return nil, err
		}

		if err := filter.ValidatePatterns(includePatterns); err != nil {
			return nil, errors.Fatalf("--include-file: %s", err)
		}

		opts.Includes = append(opts.Includes, includePatterns...)
	}

	if len(opts.InsensitiveIncludeFiles) > 0 {
		includePatterns, err := readPatternsFromFiles(opts.InsensitiveIncludeFiles)
		if err != nil {
			return nil, err
		}

		if err := filter.ValidatePatterns(includePatterns); err != nil {
			return nil, errors.Fatalf("--iinclude-file: %s", err)
		}

		opts.InsensitiveIncludes = append(opts.InsensitiveIncludes, includePatterns...)
	}

	if len(opts.InsensitiveIncludes) > 0 {
		if err := filter.ValidatePatterns(opts.InsensitiveIncludes); err != nil {
			return nil, errors.Fatalf("--iinclude: %s", err)
		}

		fs = append(fs, includeByInsensitivePattern(opts.InsensitiveIncludes))
	}

	if len(opts.Includes) > 0 {
		if err := filter.ValidatePatterns(opts.Includes); err != nil {
			return nil, errors.Fatalf("--include: %s", err)
		}

		fs = append(fs, includeByPattern(opts.Includes))
	}
	return fs, nil
}

// includeByPattern returns a IncludeByNameFunc which includes files that match
// one of the patterns.
func includeByPattern(patterns []string) IncludeByNameFunc {
	parsedPatterns := filter.ParsePatterns(patterns)
	return func(item string) (matched bool, childMayMatch bool) {
		matched, childMayMatch, err := filter.ListWithChild(parsedPatterns, item)
		if err != nil {
			Warnf("error for include pattern: %v", err)
		}

		return matched, childMayMatch
	}
}

// includeByInsensitivePattern returns a IncludeByNameFunc which includes files that match
// one of the patterns, ignoring the casing of the filenames.
func includeByInsensitivePattern(patterns []string) IncludeByNameFunc {
	for index, path := range patterns {
		patterns[index] = strings.ToLower(path)
	}

	includeFunc := includeByPattern(patterns)
	return func(item string) (matched bool, childMayMatch bool) {
		return includeFunc(strings.ToLower(item))
	}
}
