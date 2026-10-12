package filter

import (
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
)

// Glob returns the names of all files matching the pattern, supporting **
// wildcards for recursive directory matching. If the pattern contains no **,
// it delegates to filepath.Glob. Otherwise, it walks the filesystem from the
// longest static prefix directory and uses filter.Match to find matches.
//
// The returned matches are sorted in lexical order, consistent with
// filepath.Glob behavior. If no files match, a nil slice is returned with no
// error (also consistent with filepath.Glob).
func Glob(pattern string) ([]string, error) {
	if !strings.Contains(pattern, "**") {
		return filepath.Glob(pattern)
	}

	root := staticPrefix(pattern)

	var matches []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		matched, matchErr := Match(pattern, path)
		if matchErr != nil {
			return matchErr
		}
		if matched {
			matches = append(matches, path)
		}

		if d.IsDir() && path != root {
			childMatched, childErr := ChildMatch(pattern, path)
			if childErr != nil {
				return childErr
			}
			if !childMatched {
				return fs.SkipDir
			}
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Strings(matches)
	return matches, nil
}

// staticPrefix extracts the longest directory path before the first path
// component containing a wildcard character. For example, given
// "/home/user/**/*.json", it returns "/home/user". If no static prefix exists
// (e.g., "**/*.go"), it returns ".".
func staticPrefix(pattern string) string {
	// Clean the pattern to normalize separators
	pattern = filepath.Clean(pattern)
	parts := strings.Split(filepath.ToSlash(pattern), "/")

	var prefix []string
	for _, part := range parts {
		if strings.ContainsAny(part, "*?[") {
			break
		}
		prefix = append(prefix, part)
	}

	if len(prefix) == 0 {
		return "."
	}

	result := filepath.FromSlash(strings.Join(prefix, "/"))
	if result == "" {
		return "."
	}
	return result
}
