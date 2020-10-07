package filter

import (
	"path/filepath"
	"strings"

	"github.com/restic/restic/internal/errors"
)

// ErrBadString is returned when Match is called with the empty string as the
// second argument.
var ErrBadString = errors.New("filter.Match: string is empty")

// Pattern represents a preparsed filter pattern
type Pattern []string

func prepareStr(str string) ([]string, error) {
	if str == "" {
		return nil, ErrBadString
	}

	// convert file path separator to '/'
	if filepath.Separator != '/' {
		str = strings.Replace(str, string(filepath.Separator), "/", -1)
	}

	return strings.Split(str, "/"), nil
}

func preparePattern(pattern string) Pattern {
	pattern = filepath.Clean(pattern)

	// convert file path separator to '/'
	if filepath.Separator != '/' {
		pattern = strings.Replace(pattern, string(filepath.Separator), "/", -1)
	}

	return strings.Split(pattern, "/")
}

// Match returns true if str matches the pattern. When the pattern is
// malformed, filepath.ErrBadPattern is returned. The empty pattern matches
// everything, when str is the empty string ErrBadString is returned.
//
// Pattern can be a combination of patterns suitable for filepath.Match, joined
// by filepath.Separator.
//
// In addition patterns suitable for filepath.Match, pattern accepts a
// recursive wildcard '**', which greedily matches an arbitrary number of
// intermediate directories.
func Match(pattern, str string) (matched bool, err error) {
	if pattern == "" {
		return true, nil
	}

	patterns := preparePattern(pattern)
	strs, err := prepareStr(str)

	if err != nil {
		return false, err
	}

	return match(patterns, strs)
}

// ChildMatch returns true if children of str can match the pattern. When the pattern is
// malformed, filepath.ErrBadPattern is returned. The empty pattern matches
// everything, when str is the empty string ErrBadString is returned.
//
// Pattern can be a combination of patterns suitable for filepath.Match, joined
// by filepath.Separator.
//
// In addition patterns suitable for filepath.Match, pattern accepts a
// recursive wildcard '**', which greedily matches an arbitrary number of
// intermediate directories.
func ChildMatch(pattern, str string) (matched bool, err error) {
	if pattern == "" {
		return true, nil
	}

	patterns := preparePattern(pattern)
	strs, err := prepareStr(str)

	if err != nil {
		return false, err
	}

	return childMatch(patterns, strs)
}

func childMatch(patterns Pattern, strs []string) (matched bool, err error) {
	if patterns[0] != "" {
		// relative pattern can always be nested down
		return true, nil
	}

	ok, pos := hasDoubleWildcard(patterns)
	if ok && len(strs) >= pos {
		// cut off at the double wildcard
		strs = strs[:pos]
	}

	// match path against absolute pattern prefix
	l := 0
	if len(strs) > len(patterns) {
		l = len(patterns)
	} else {
		l = len(strs)
	}
	return match(patterns[0:l], strs)
}

func hasDoubleWildcard(list Pattern) (ok bool, pos int) {
	for i, item := range list {
		if item == "**" {
			return true, i
		}
	}

	return false, 0
}

func match(patterns Pattern, strs []string) (matched bool, err error) {
	if ok, pos := hasDoubleWildcard(patterns); ok {
		// gradually expand '**' into separate wildcards
		newPat := make(Pattern, len(strs))
		// copy static prefix once
		copy(newPat, patterns[:pos])
		for i := 0; i <= len(strs)-len(patterns)+1; i++ {
			// limit to static prefix and already appended '*'
			newPat := newPat[:pos+i]
			// in the first iteration the wildcard expands to nothing
			if i > 0 {
				newPat[pos+i-1] = "*"
			}
			newPat = append(newPat, patterns[pos+1:]...)

			matched, err := match(newPat, strs)
			if err != nil {
				return false, err
			}

			if matched {
				return true, nil
			}
		}

		return false, nil
	}

	if len(patterns) == 0 && len(strs) == 0 {
		return true, nil
	}

	if len(patterns) <= len(strs) {
		maxOffset := len(strs) - len(patterns)
		// special case absolute patterns
		if patterns[0] == "" {
			maxOffset = 0
		}
	outer:
		for offset := maxOffset; offset >= 0; offset-- {

			for i := len(patterns) - 1; i >= 0; i-- {
				ok, err := filepath.Match(patterns[i], strs[offset+i])
				if err != nil {
					return false, errors.Wrap(err, "Match")
				}

				if !ok {
					continue outer
				}
			}

			return true, nil
		}
	}

	return false, nil
}

// ParsePatterns prepares a list of patterns for use with List.
func ParsePatterns(patterns []string) []Pattern {
	patpat := make([]Pattern, 0)
	for _, pat := range patterns {
		if pat == "" {
			continue
		}

		pats := preparePattern(pat)
		patpat = append(patpat, pats)
	}
	return patpat
}

// List returns true if str matches one of the patterns. Empty patterns are ignored.
func List(patterns []Pattern, str string) (matched bool, err error) {
	matched, _, err = list(patterns, false, str)
	return matched, err
}

// ListWithChild returns true if str matches one of the patterns. Empty patterns are ignored.
func ListWithChild(patterns []Pattern, str string) (matched bool, childMayMatch bool, err error) {
	return list(patterns, true, str)
}

// List returns true if str matches one of the patterns. Empty patterns are ignored.
func list(patterns []Pattern, checkChildMatches bool, str string) (matched bool, childMayMatch bool, err error) {
	if len(patterns) == 0 {
		return false, false, nil
	}

	strs, err := prepareStr(str)
	if err != nil {
		return false, false, err
	}
	for _, pat := range patterns {
		m, err := match(pat, strs)
		if err != nil {
			return false, false, err
		}

		var c bool
		if checkChildMatches {
			c, err = childMatch(pat, strs)
			if err != nil {
				return false, false, err
			}
		} else {
			c = true
		}

		matched = matched || m
		childMayMatch = childMayMatch || c

		if matched && childMayMatch {
			return true, true, nil
		}
	}

	return matched, childMayMatch, nil
}
