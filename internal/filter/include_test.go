package filter

import (
	"testing"
)

func TestIncludeByPattern(t *testing.T) {
	var tests = []struct {
		filename string
		include  bool
	}{
		{filename: "/home/user/foo.go", include: true},
		{filename: "/home/user/foo.c", include: false},
		{filename: "/home/user/foobar", include: false},
		{filename: "/home/user/foobar/x", include: false},
		{filename: "/home/user/README", include: false},
		{filename: "/home/user/README.md", include: true},
	}

	patterns := []string{"*.go", "README.md"}

	for _, tc := range tests {
		t.Run(tc.filename, func(t *testing.T) {
			includeFunc := IncludeByPattern(patterns, nil)
			matched, _ := includeFunc(tc.filename)
			if matched != tc.include {
				t.Fatalf("wrong result for filename %v: want %v, got %v",
					tc.filename, tc.include, matched)
			}
		})
	}
}

func TestIncludeByLiteralPattern(t *testing.T) {
	// Literal patterns: every char (including glob metacharacters) must be
	// matched verbatim. This is the matcher behind --include-from-raw, used
	// when the caller already has exact path strings and cannot represent
	// '[' or ']' as a glob (no portable escape — see filter.LiteralPattern).
	patterns := []string{
		"/home/user/Q3 [draft].pdf",
		"/home/user/data/*.txt", // a literal asterisk and dot, not a glob
		"/home/user/README.md",
	}

	var tests = []struct {
		filename      string
		include       bool
		childMayMatch bool
	}{
		// Brackets matched literally — the same pattern under
		// IncludeByPattern would be a character class.
		{filename: "/home/user/Q3 [draft].pdf", include: true, childMayMatch: true},
		// Character-class interpretation would match these, the literal
		// interpretation must not.
		{filename: "/home/user/Q3 d.pdf", include: false, childMayMatch: false},
		{filename: "/home/user/Q3 draft.pdf", include: false, childMayMatch: false},
		// '*' matched literally — only the exact filename containing the
		// asterisk matches, every other .txt does not.
		{filename: "/home/user/data/*.txt", include: true, childMayMatch: true},
		{filename: "/home/user/data/foo.txt", include: false, childMayMatch: false},
		// Plain entry still works alongside the glob-containing ones.
		{filename: "/home/user/README.md", include: true, childMayMatch: true},
		{filename: "/home/user/README", include: false, childMayMatch: false},
		// Parent directories report childMayMatch so the restorer keeps
		// descending into them.
		{filename: "/home/user", include: false, childMayMatch: true},
		{filename: "/home/user/data", include: false, childMayMatch: true},
		// Unrelated path — no match, no child match.
		{filename: "/etc/passwd", include: false, childMayMatch: false},
	}

	includeFunc := IncludeByLiteralPattern(patterns)
	for _, tc := range tests {
		t.Run(tc.filename, func(t *testing.T) {
			matched, child := includeFunc(tc.filename)
			if matched != tc.include {
				t.Errorf("matched: want %v, got %v", tc.include, matched)
			}
			if child != tc.childMayMatch {
				t.Errorf("childMayMatch: want %v, got %v", tc.childMayMatch, child)
			}
		})
	}
}

func TestIncludeByInsensitivePattern(t *testing.T) {
	var tests = []struct {
		filename string
		include  bool
	}{
		{filename: "/home/user/foo.GO", include: true},
		{filename: "/home/user/foo.c", include: false},
		{filename: "/home/user/foobar", include: false},
		{filename: "/home/user/FOObar/x", include: false},
		{filename: "/home/user/README", include: false},
		{filename: "/home/user/readme.MD", include: true},
	}

	patterns := []string{"*.go", "README.md"}

	for _, tc := range tests {
		t.Run(tc.filename, func(t *testing.T) {
			includeFunc := IncludeByInsensitivePattern(patterns, nil)
			matched, _ := includeFunc(tc.filename)
			if matched != tc.include {
				t.Fatalf("wrong result for filename %v: want %v, got %v",
					tc.filename, tc.include, matched)
			}
		})
	}
}
