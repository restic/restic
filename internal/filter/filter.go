package filter

import (
	"path/filepath"
	"strings"

	"github.com/restic/restic/internal/errors"
)

// ErrBadString is returned when Match is called with the empty string as the
// second argument.
var ErrBadString = errors.New("filter.Match: string is empty")

type patternPart struct {
	pattern  string // First is "/" for absolute pattern; "" for "**".
	isSimple bool
}

// Pattern represents a preparsed filter pattern
type Pattern struct {
	original  string
	parts     []patternPart
	isNegated bool
}

func prepareStr(str string) ([]string, error) {
	if str == "" {
		return nil, ErrBadString
	}
	return splitPath(str), nil
}

func preparePattern(patternStr string) Pattern {
	var negate bool

	originalPattern := patternStr

	if patternStr[0] == '!' {
		negate = true
		patternStr = patternStr[1:]
	}

	pathParts := splitPath(filepath.Clean(patternStr))
	parts := make([]patternPart, len(pathParts))
	for i, part := range pathParts {
		isSimple := !strings.ContainsAny(part, "\\[]*?")
		// Replace "**" with the empty string to get faster comparisons
		// (length-check only) in hasDoubleWildcard.
		if part == "**" {
			part = ""
		}
		parts[i] = patternPart{part, isSimple}
	}

	return Pattern{originalPattern, parts, negate}
}

// Split p into path components. Assuming p has been Cleaned, no component
// will be empty. For absolute paths, the first component is "/".
func splitPath(p string) []string {
	parts := strings.Split(filepath.ToSlash(p), "/")
	if parts[0] == "" {
		parts[0] = "/"
	}
	return parts
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
func Match(patternStr, str string) (matched bool, err error) {
	if patternStr == "" {
		return true, nil
	}

	pattern := preparePattern(patternStr)
	strs, err := prepareStr(str)

	if err != nil {
		return false, err
	}

	return match(pattern, strs)
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
func ChildMatch(patternStr, str string) (matched bool, err error) {
	if patternStr == "" {
		return true, nil
	}

	pattern := preparePattern(patternStr)
	strs, err := prepareStr(str)

	if err != nil {
		return false, err
	}

	return childMatch(pattern, strs)
}

func childMatch(pattern Pattern, strs []string) (matched bool, err error) {
	if pattern.parts[0].pattern != "/" {
		// relative pattern can always be nested down
		return true, nil
	}

	ok, pos := hasDoubleWildcard(pattern)
	if ok && len(strs) >= pos {
		// cut off at the double wildcard
		strs = strs[:pos]
	}

	// match path against absolute pattern prefix
	l := 0
	if len(strs) > len(pattern.parts) {
		l = len(pattern.parts)
	} else {
		l = len(strs)
	}
	return match(Pattern{pattern.original, pattern.parts[0:l], pattern.isNegated}, strs)
}

func hasDoubleWildcard(list Pattern) (ok bool, pos int) {
	for i, item := range list.parts {
		if item.pattern == "" {
			return true, i
		}
	}

	return false, 0
}

func match(pattern Pattern, strs []string) (matched bool, err error) {
	if ok, pos := hasDoubleWildcard(pattern); ok {
		// gradually expand '**' into separate wildcards
		newPat := make([]patternPart, len(strs))
		// copy static prefix once
		copy(newPat, pattern.parts[:pos])
		for i := 0; i <= len(strs)-len(pattern.parts)+1; i++ {
			// limit to static prefix and already appended '*'
			newPat := newPat[:pos+i]
			// in the first iteration the wildcard expands to nothing
			if i > 0 {
				newPat[pos+i-1] = patternPart{"*", false}
			}
			newPat = append(newPat, pattern.parts[pos+1:]...)

			matched, err := match(Pattern{pattern.original, newPat, pattern.isNegated}, strs)
			if err != nil {
				return false, err
			}

			if matched {
				return true, nil
			}
		}

		return false, nil
	}

	if len(pattern.parts) == 0 && len(strs) == 0 {
		return true, nil
	}

	// an empty pattern never matches a non-empty path
	if len(pattern.parts) == 0 {
		return false, nil
	}

	if len(pattern.parts) <= len(strs) {
		minOffset := 0
		maxOffset := len(strs) - len(pattern.parts)
		// special case absolute patterns
		if pattern.parts[0].pattern == "/" {
			maxOffset = 0
		} else if strs[0] == "/" {
			// skip absolute path marker if pattern is not rooted
			minOffset = 1
		}
	outer:
		for offset := maxOffset; offset >= minOffset; offset-- {

			for i := len(pattern.parts) - 1; i >= 0; i-- {
				var ok bool
				if pattern.parts[i].isSimple {
					ok = pattern.parts[i].pattern == strs[offset+i]
				} else {
					ok, err = filepath.Match(pattern.parts[i].pattern, strs[offset+i])
					if err != nil {
						return false, errors.Wrap(err, "Match")
					}
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

type InvalidPatternError struct {
	InvalidPatterns []string
}

func (e *InvalidPatternError) Error() string {
	return "invalid pattern(s) provided:\n" + strings.Join(e.InvalidPatterns, "\n")
}

// ValidatePatterns validates a slice of patterns.
// Returns true if all patterns are valid - false otherwise, along with the invalid patterns.
func ValidatePatterns(patterns []string) error {
	invalidPatterns := make([]string, 0)

	for _, Pattern := range ParsePatterns(patterns) {
		// Validate all pattern parts
		for _, part := range Pattern.parts {
			// Validate the pattern part by trying to match it against itself
			if _, validErr := filepath.Match(part.pattern, part.pattern); validErr != nil {
				invalidPatterns = append(invalidPatterns, Pattern.original)

				// If a single part is invalid, stop processing this pattern
				continue
			}
		}
	}

	if len(invalidPatterns) > 0 {
		return &InvalidPatternError{InvalidPatterns: invalidPatterns}
	}
	return nil
}

// ParsePatterns prepares a list of patterns for use with List.
func ParsePatterns(pattern []string) []Pattern {
	patpat := make([]Pattern, 0)
	for _, pat := range pattern {
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

// list returns true if str matches one of the patterns. Empty patterns are ignored.
// Patterns prefixed by "!" are negated: any matching file excluded by a previous pattern
// will become included again.
func list(patterns []Pattern, checkChildMatches bool, str string) (matched bool, childMayMatch bool, err error) {
	if len(patterns) == 0 {
		return false, false, nil
	}

	strs, err := prepareStr(str)
	if err != nil {
		return false, false, err
	}

	hasNegatedPattern := false
	for _, pat := range patterns {
		hasNegatedPattern = hasNegatedPattern || pat.isNegated
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

		if pat.isNegated {
			matched = matched && !m
			childMayMatch = childMayMatch && !m
		} else {
			matched = matched || m
			childMayMatch = childMayMatch || c

			if matched && childMayMatch && !hasNegatedPattern {
				// without negative patterns the result cannot change any more
				break
			}
		}
	}

	return matched, childMayMatch, nil
}
