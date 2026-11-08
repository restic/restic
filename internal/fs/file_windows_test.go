//go:build windows

package fs

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestFixpathTrailingSpaceAndDot verifies that fixpath does not lose a
// trailing space or dot on the final path component. Windows'
// GetFullPathNameW (invoked internally by filepath.Abs) silently strips
// trailing spaces/dots from the last path component; that stripping needs
// to happen (if at all) only on the abs-path resolution, not on the
// caller-supplied leaf name, otherwise a file or directory such as
// "dir-b " gets silently renamed to "dir-b" on restore.
//
// See https://github.com/restic/restic/issues/22059
func TestFixpathTrailingSpaceAndDot(t *testing.T) {
	base := t.TempDir()

	leaves := []string{
		"dir-b ",   // trailing space
		"dir-b.",   // trailing dot
		"dir-b  ",  // multiple trailing spaces
		"dir-b...", // multiple trailing dots
		"dir-b. ",  // mixed trailing dot+space
		"normal",   // control case: no trailing space/dot, must be untouched
	}

	for _, leaf := range leaves {
		leaf := leaf
		t.Run(leaf, func(t *testing.T) {
			input := filepath.Join(base, leaf)
			got := fixpath(input)

			if !strings.HasPrefix(got, extendedPathPrefix) {
				t.Fatalf("fixpath(%q) = %q, want \\\\?\\ prefix", input, got)
			}

			gotLeaf := filepath.Base(got)
			if gotLeaf != leaf {
				t.Fatalf("fixpath(%q) = %q, trailing space/dot on final component was not preserved: got leaf %q, want %q", input, got, gotLeaf, leaf)
			}
		})
	}
}

// Note: splitTrailingSpaceAndDot itself is pure string manipulation with no
// OS dependency, so it is unit-tested natively (on any platform, including
// this build) in pathutil_test.go rather than here. This file only covers
// the Windows-specific fixpath() integration above.
