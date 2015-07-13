package filter

import (
	"errors"
	"path/filepath"
	"strings"
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
func Match(pattern, str string) (matched bool, err error) {
	if pattern == "" {
		return true, nil
	}

	if str == "" {
		return false, ErrBadString
	}

	patterns := strings.Split(pattern, string(filepath.Separator))
	strs := strings.Split(str, string(filepath.Separator))

	return match(patterns, strs)
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
					return false, err
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

// MatchList returns true if str matches one of the patterns.
func MatchList(patterns []string, str string) (matched bool, err error) {
	for _, pat := range patterns {
		matched, err = Match(pat, str)
		if err != nil {
			return false, err
		}

		if matched {
			return true, nil
		}
	}

	return false, nil
}

// matchList returns true if str matches one of the patterns.
func matchList(patterns [][]string, str []string) (matched bool, err error) {
	for _, pat := range patterns {
		matched, err = match(pat, str)
		if err != nil {
			return false, err
		}

		if matched {
			return true, nil
		}
	}

	return false, nil
}

// Filter contains include and exclude patterns. If both lists of patterns are
// empty, all files are accepted.
type Filter struct {
	include, exclude [][]string
}

// New returns a new filter with the given include/exclude lists of patterns.
func New(include, exclude []string) *Filter {
	f := &Filter{}

	for _, pat := range include {
		f.include = append(f.include, strings.Split(pat, string(filepath.Separator)))
	}

	for _, pat := range exclude {
		f.exclude = append(f.exclude, strings.Split(pat, string(filepath.Separator)))
	}

	return f
}

// Match tests a filename against the filter. If include and exclude patterns
// are both empty, true is returned.
//
// If only include patterns and no exclude patterns are configured, true is
// returned iff name matches one of the include patterns.
//
// If only exclude patterns and no include patterns are configured, true is
// returned iff name does not match all of the exclude patterns.
func (f Filter) Match(name string) (matched bool, err error) {
	if name == "" {
		return false, ErrBadString
	}

	if len(f.include) == 0 && len(f.exclude) == 0 {
		return true, nil
	}

	names := strings.Split(name, string(filepath.Separator))
	if len(f.exclude) == 0 {
		return matchList(f.include, names)
	}

	if len(f.include) == 0 {
		match, err := matchList(f.exclude, names)
		return !match, err
	}

	excluded, err := matchList(f.exclude, names)
	if err != nil {
		return false, err
	}

	if !excluded {
		return true, nil
	}

	return matchList(f.include, names)
}
