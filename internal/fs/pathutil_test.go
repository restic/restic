package fs

import "testing"

// TestSplitTrailingSpaceAndDot tests the helper used by fixpath (see
// file_windows.go) in isolation, including the edge case of a component
// made up entirely of dots (e.g. "..", "."), which must be left untouched
// since it is a navigational path segment rather than a literal filename.
//
// This is plain string logic with no OS dependency, so it is exercised
// natively here (in addition to the Windows-specific fixpath() integration
// test in file_windows_test.go, which requires GOOS=windows).
func TestSplitTrailingSpaceAndDot(t *testing.T) {
	cases := []struct {
		name       string
		wantBase   string
		wantSuffix string
	}{
		{`C:\foo\bar `, `C:\foo\bar`, ` `},
		{`C:\foo\bar.`, `C:\foo\bar`, `.`},
		{`C:\foo\bar...`, `C:\foo\bar`, `...`},
		{`C:\foo\bar`, `C:\foo\bar`, ``},
		{`C:\foo\..`, `C:\foo\..`, ``},
		{`C:\foo\.`, `C:\foo\.`, ``},
		{`C:\foo\   `, `C:\foo\   `, ``},
		{`bar `, `bar`, ` `},
		{`/foo/bar.`, `/foo/bar`, `.`},
		{``, ``, ``},
	}

	for _, c := range cases {
		gotBase, gotSuffix := splitTrailingSpaceAndDot(c.name)
		if gotBase != c.wantBase || gotSuffix != c.wantSuffix {
			t.Errorf("splitTrailingSpaceAndDot(%q) = (%q, %q), want (%q, %q)",
				c.name, gotBase, gotSuffix, c.wantBase, c.wantSuffix)
		}
		if gotBase+gotSuffix != c.name {
			t.Errorf("splitTrailingSpaceAndDot(%q): base+suffix = %q, want original %q", c.name, gotBase+gotSuffix, c.name)
		}
	}
}
