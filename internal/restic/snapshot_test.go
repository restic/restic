package restic

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	rtest "github.com/restic/restic/internal/test"
)

func TestNewSnapshot(t *testing.T) {
	paths := []string{"/home/foobar"}

	_, err := NewSnapshot(paths, nil, "foo", time.Now(), "", 0)
	rtest.OK(t, err)
}

func TestPathSplit(t *testing.T) {
	var tests = []struct {
		Input     string
		Root      string
		Remainder []string
	}{
		{
			"foo",
			"",
			[]string{"foo"},
		},
		{
			"foo/bar",
			"",
			[]string{"foo", "bar"},
		},
		{
			"/home/user/work",
			"/",
			[]string{"home", "user", "work"},
		},
	}

	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			root, rest := pathSplit(test.Input)
			if root != test.Root {
				t.Errorf("wrong root path returned, want %q, got %q", test.Root, root)
			}

			if !cmp.Equal(test.Remainder, rest) {
				t.Fatal(cmp.Diff(test.Remainder, rest))
			}
		})
	}
}

func TestPathSplitPrefix(t *testing.T) {
	mustAbs := func (path string) (ret string) {
		ret, _ = filepath.Abs(path)
		return
	}
	var tests = []struct {
		Path   string
		Prefix string
		Strip  int
		Result string
	}{
		{
			Path:   "foo",
			Result: mustAbs("foo"),
		},
		{
			Path:   "/home/user",
			Result: "/home/user",
		},
		{
			Path:   "../home/user",
			Result: mustAbs("../home/user"),
		},
		{
			Path:   "foo/bar",
			Strip:  1,
			Result: "bar",
		},
		{
			Path:   "foo/bar",
			Strip:  2,
			Result: ".",
		},
		{
			Path:   "foo/bar",
			Strip:  3,
			Result: ".",
		},
		{
			Path:   "/home/user/foo/bar",
			Strip:  1,
			Result: "user/foo/bar",
		},
		{
			Path:   "/home/user/foo/bar",
			Strip:  2,
			Result: "foo/bar",
		},
		{
			Path:   "/home/user/foo/bar",
			Strip:  3,
			Result: "bar",
		},
		{
			Path:   "/home/user/foo/bar",
			Prefix: "/srv/backup",
			Result: "/srv/backup/home/user/foo/bar",
		},
		{
			Path:   "../user/foo/bar",
			Prefix: "/srv/backup",
			Result: "/srv/user/foo/bar",
		},
		{
			Path:   "/home/user/foo/bar",
			Strip:  1,
			Prefix: "/srv/backup",
			Result: "/srv/backup/user/foo/bar",
		},
		{
			Path:   "/home/user/foo/bar",
			Strip:  2,
			Prefix: "/srv/backup",
			Result: "/srv/backup/foo/bar",
		},
		{
			Path:   "../user/foo/bar",
			Strip:  1,
			Prefix: "/srv/backup",
			Result: "/srv/backup/user/foo/bar",
		},
	}

	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			res := pathStripPrefix(test.Path, test.Prefix, test.Strip)
			if res != test.Result {
				t.Fatalf("wrong result, want %q, got %q", test.Result, res)
			}
		})
	}
}
