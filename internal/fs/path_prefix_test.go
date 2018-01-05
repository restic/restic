package fs

import (
	"path/filepath"
	"runtime"
	"testing"
)

func fromSlashAbs(p string) string {
	if runtime.GOOS == "windows" {
		if len(p) > 0 && p[0] == '/' {
			p = "c:" + p
		}
	}

	return filepath.FromSlash(p)
}

func TestHasPathPrefix(t *testing.T) {
	var tests = []struct {
		base, p string
		result  bool
	}{
		{"", "", true},
		{".", ".", true},
		{".", "foo", true},
		{"foo", ".", false},
		{"/", "", false},
		{"/", "x", false},
		{"x", "/", false},
		{"/", "/x", true},
		{"/x", "/y", false},
		{"/home/user/foo", "/home", false},
		{"/home/user/foo/", "/home", false},
		{"/home/user/foo", "/home/", false},
		{"/home/user/foo/", "/home/", false},
		{"/home/user/foo", "/home/user/foo/bar", true},
		{"/home/user/foo", "/home/user/foo/bar/baz/x/y/z", true},
		{"/home/user/foo", "/home/user/foobar", false},
		{"/home/user/Foo", "/home/user/foo/bar/baz", false},
		{"/home/user/foo", "/home/user/Foo/bar/baz", false},
		{"user/foo", "user/foo/bar/baz", true},
		{"user/foo", "./user/foo", true},
		{"user/foo", "./user/foo/", true},
		{"/home/user/foo", "./user/foo/", false},
	}

	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			base := fromSlashAbs(test.base)
			p := fromSlashAbs(test.p)
			result := HasPathPrefix(base, p)
			if result != test.result {
				t.Fatalf("wrong result for HasPathPrefix(%q, %q): want %v, got %v",
					base, p, test.result, result)
			}
		})
	}
}
