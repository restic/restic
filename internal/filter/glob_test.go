package filter_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/restic/restic/internal/filter"
	rtest "github.com/restic/restic/internal/test"
)

func createGlobTestDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	dirs := []string{
		"sub",
		"sub/deep",
		filepath.Join("sub", "deep", "nested"),
	}
	for _, d := range dirs {
		err := os.MkdirAll(filepath.Join(dir, d), 0o755)
		rtest.OK(t, err)
	}

	files := []string{
		"file.txt",
		"file.json",
		filepath.Join("sub", "file.txt"),
		filepath.Join("sub", "deep", "file.txt"),
		filepath.Join("sub", "deep", "other.json"),
		filepath.Join("sub", "deep", "nested", "file.txt"),
	}
	for _, f := range files {
		err := os.WriteFile(filepath.Join(dir, f), []byte("test"), 0o644)
		rtest.OK(t, err)
	}

	return dir
}

func TestGlobDoublestar(t *testing.T) {
	dir := createGlobTestDir(t)

	tests := []struct {
		pattern  string
		expected []string
	}{
		{
			pattern: filepath.Join(dir, "**", "*.txt"),
			expected: []string{
				filepath.Join(dir, "file.txt"),
				filepath.Join(dir, "sub", "deep", "file.txt"),
				filepath.Join(dir, "sub", "deep", "nested", "file.txt"),
				filepath.Join(dir, "sub", "file.txt"),
			},
		},
		{
			pattern: filepath.Join(dir, "**", "*.json"),
			expected: []string{
				filepath.Join(dir, "file.json"),
				filepath.Join(dir, "sub", "deep", "other.json"),
			},
		},
		{
			pattern: filepath.Join(dir, "sub", "**", "*.txt"),
			expected: []string{
				filepath.Join(dir, "sub", "deep", "file.txt"),
				filepath.Join(dir, "sub", "deep", "nested", "file.txt"),
				filepath.Join(dir, "sub", "file.txt"),
			},
		},
		{
			pattern: filepath.Join(dir, "**", "deep", "*.txt"),
			expected: []string{
				filepath.Join(dir, "sub", "deep", "file.txt"),
			},
		},
		{
			pattern:  filepath.Join(dir, "**", "*.py"),
			expected: nil,
		},
	}

	for _, test := range tests {
		t.Run(test.pattern, func(t *testing.T) {
			matches, err := filter.Glob(test.pattern)
			rtest.OK(t, err)
			rtest.Equals(t, test.expected, matches)
		})
	}
}

func TestGlobNoDoublestar(t *testing.T) {
	dir := createGlobTestDir(t)

	// Without **, should delegate to filepath.Glob and only match top-level
	pattern := filepath.Join(dir, "*.txt")
	matches, err := filter.Glob(pattern)
	rtest.OK(t, err)

	expected := []string{filepath.Join(dir, "file.txt")}
	rtest.Equals(t, expected, matches)
}

func TestGlobNoMatches(t *testing.T) {
	dir := createGlobTestDir(t)

	matches, err := filter.Glob(filepath.Join(dir, "**", "*.xyz"))
	rtest.OK(t, err)
	rtest.Equals(t, ([]string)(nil), matches)
}
