//go:build windows

package filter_test

import "testing"

// Patterns for UNC paths are not changed by filepath.Clean in a way that
// involves a `.\` prefix, so they must keep matching as before.
var uncMatchTests = []struct {
	pattern string
	path    string
	match   bool
}{
	{`\\server\share\foo`, `\\server\share\foo\bar`, true},
	{`\\server\share\foo`, `\\server\share\foo`, true},
	{`//server/share/foo`, `\\server\share\foo\bar`, true},
	{`\\server\share\foo`, `\\server\other\foo\bar`, false},
	{`\\server\share\foo`, `\\other\share\foo\bar`, false},
	{`\\server\share\*\test.go`, `\\server\share\foo\test.go`, true},
	{`\\server\share\**\test.go`, `\\server\share\foo\bar\test.go`, true},
	{`\\server\[sS]hare\foo`, `\\server\Share\foo\bar`, true},
	{`\\?\C:\foo`, `\\?\C:\foo\bar`, true},
	{`\\?\UNC\server\share\foo`, `\\?\UNC\server\share\foo\bar`, true},
	{`[cC]:\foo`, `\\server\share\foo\bar`, false},
	{`[cC]:/foo`, `\\server\share\foo\bar`, false},
}

func TestMatchUNCPaths(t *testing.T) {
	for _, test := range uncMatchTests {
		t.Run("", func(t *testing.T) {
			testpattern(t, test.pattern, test.path, test.match)
		})
	}
}
