package frontend

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/restic/restic/internal/fs"
)

func TestPathComponents(t *testing.T) {
	var tests = []struct {
		p       string
		c       []string
		virtual bool
		rel     bool
		win     bool
	}{
		{
			p: "/foo/bar/baz",
			c: []string{"foo", "bar", "baz"},
		},
		{
			p:   "/foo/bar/baz",
			c:   []string{"foo", "bar", "baz"},
			rel: true,
		},
		{
			p: "foo/bar/baz",
			c: []string{"foo", "bar", "baz"},
		},
		{
			p:   "foo/bar/baz",
			c:   []string{"foo", "bar", "baz"},
			rel: true,
		},
		{
			p: "../foo/bar/baz",
			c: []string{"foo", "bar", "baz"},
		},
		{
			p:   "../foo/bar/baz",
			c:   []string{"..", "foo", "bar", "baz"},
			rel: true,
		},
		{
			p:       "c:/foo/bar/baz",
			c:       []string{"c", "foo", "bar", "baz"},
			virtual: true,
			rel:     true,
			win:     true,
		},
		{
			p:       "c:/foo/../bar/baz",
			c:       []string{"c", "bar", "baz"},
			virtual: true,
			win:     true,
		},
		{
			p:       `c:\foo\..\bar\baz`,
			c:       []string{"c", "bar", "baz"},
			virtual: true,
			win:     true,
		},
		{
			p:       "c:/foo/../bar/baz",
			c:       []string{"c", "bar", "baz"},
			virtual: true,
			rel:     true,
			win:     true,
		},
		{
			p:       `c:\foo\..\bar\baz`,
			c:       []string{"c", "bar", "baz"},
			virtual: true,
			rel:     true,
			win:     true,
		},
	}

	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			if test.win && runtime.GOOS != "windows" {
				t.Skip("skip test on unix")
			}

			c, v := pathComponents(fs.Local{}, filepath.FromSlash(test.p), test.rel)
			if !cmp.Equal(test.c, c) {
				t.Error(test.c, c)
			}

			if v != test.virtual {
				t.Errorf("unexpected virtual prefix count returned, want %v, got %v", test.virtual, v)
			}
		})
	}
}

func TestRootDirectory(t *testing.T) {
	var tests = []struct {
		target string
		root   string
		unix   bool
		win    bool
	}{
		{target: ".", root: "."},
		{target: "foo/bar/baz", root: "."},
		{target: "../foo/bar/baz", root: ".."},
		{target: "..", root: ".."},
		{target: "../../..", root: "../../.."},
		{target: "/home/foo", root: "/", unix: true},
		{target: "c:/home/foo", root: "c:/", win: true},
		{target: `c:\home\foo`, root: `c:\`, win: true},
		{target: "//host/share/foo", root: "//host/share/", win: true},
	}

	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			if test.unix && runtime.GOOS == "windows" {
				t.Skip("skip test on windows")
			}
			if test.win && runtime.GOOS != "windows" {
				t.Skip("skip test on unix")
			}

			root := rootDirectory(fs.Local{}, filepath.FromSlash(test.target))
			want := filepath.FromSlash(test.root)
			if root != want {
				t.Fatalf("wrong root directory, want %v, got %v", want, root)
			}
		})
	}
}
