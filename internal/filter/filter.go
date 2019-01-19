package filter

import (
	"path/filepath"
	"strings"

	"github.com/restic/restic/internal/errors"
)

// ErrBadString is returned when Match is called with the empty string as the
// second argument.
var ErrBadString = errors.New("filter.Match: string is empty")

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

	pattern = filepath.Clean(pattern)

	if str == "" {
		return false, ErrBadString
	}

	// convert file path separator to '/'
	if filepath.Separator != '/' {
		pattern = strings.Replace(pattern, string(filepath.Separator), "/", -1)
		str = strings.Replace(str, string(filepath.Separator), "/", -1)
	}

	patterns := strings.Split(pattern, "/")
	strs := strings.Split(str, "/")

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

	pattern = filepath.Clean(pattern)

	if str == "" {
		return false, ErrBadString
	}

	// convert file path separator to '/'
	if filepath.Separator != '/' {
		pattern = strings.Replace(pattern, string(filepath.Separator), "/", -1)
		str = strings.Replace(str, string(filepath.Separator), "/", -1)
	}

	patterns := strings.Split(pattern, "/")
	strs := strings.Split(str, "/")

	return childMatch(patterns, strs)
}

func childMatch(patterns, strs []string) (matched bool, err error) {
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

func hasDoubleWildcard(list []string) (ok bool, pos int) {
	for i, item := range list {
		if item == "**" {
			return true, i
		}
	}

	return false, 0
}

func match(patterns, strs []string) (matched bool, err error) {
	if ok, pos := hasDoubleWildcard(patterns); ok {
		// gradually expand '**' into separate wildcards
		for i := 0; i <= len(strs)-len(patterns)+1; i++ {
			newPat := make([]string, pos)
			copy(newPat, patterns[:pos])
			for k := 0; k < i; k++ {
				newPat = append(newPat, "*")
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
	outer:
		for offset := len(strs) - len(patterns); offset >= 0; offset-- {

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

// List returns true if str matches one of the patterns. Empty patterns are
// ignored.
func List(patterns []string, str string) (matched bool, childMayMatch bool, err error) {
	for _, pat := range patterns {
		if pat == "" {
			continue
		}

		m, err := Match(pat, str)
		if err != nil {
			return false, false, err
		}

		c, err := ChildMatch(pat, str)
		if err != nil {
			return false, false, err
		}

		matched = matched || m
		childMayMatch = childMayMatch || c

		if matched && childMayMatch {
			return true, true, nil
		}
	}

	return matched, childMayMatch, nil
}
