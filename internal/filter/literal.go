package filter

import (
	"path/filepath"
)

// LiteralPattern preparses patternStr as a literal-path pattern: every path
// component is matched verbatim with ==, glob metacharacters lose their
// special meaning, and `**` is not expanded as a recursive wildcard. This is
// the matcher behind --include-from-raw.
//
// Use LiteralPattern when the caller already has exact path strings (read
// from a NUL-separated file, returned by another tool, ...) and wants to
// avoid the shell-glob round-trip. The most common reason is that the path
// contains characters with no portable glob escape — on Windows, Go's
// path/filepath.Match treats '\' as a path separator (not an escape) and
// rejects the POSIX class self-escape `[[]` / `[]]` as an invalid pattern,
// so a literal '[' or ']' is otherwise unrepresentable in a glob.
//
// Negation prefix '!' is NOT stripped by LiteralPattern: if the file you
// want to match starts with a literal '!' character, write it as '!' and
// LiteralPattern will match it as such.
func LiteralPattern(patternStr string) Pattern {
	pathParts := splitPath(filepath.Clean(patternStr))
	parts := make([]patternPart, len(pathParts))
	for i, part := range pathParts {
		// Force isSimple = true so match() takes the == equality branch
		// instead of calling filepath.Match. This is the whole point of
		// the literal mode: never invoke the glob engine.
		parts[i] = patternPart{part, true}
	}
	return Pattern{patternStr, parts, false}
}

// ParseLiteralPatterns prepares a list of literal patterns for use with
// List / ListWithChild. See LiteralPattern for semantics. Empty patterns
// are skipped (mirrors ParsePatterns).
func ParseLiteralPatterns(patterns []string) []Pattern {
	out := make([]Pattern, 0, len(patterns))
	for _, p := range patterns {
		if p == "" {
			continue
		}
		out = append(out, LiteralPattern(p))
	}
	return out
}
