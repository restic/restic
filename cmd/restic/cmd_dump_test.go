package main

import (
	"testing"

	rtest "github.com/restic/restic/internal/test"
)

func TestDumpSplitPath(t *testing.T) {
	testPaths := []struct {
		path   string
		result []string
	}{
		{"", []string{""}},
		{"test", []string{"test"}},
		{"test/dir", []string{"test", "dir"}},
		{"test/dir/sub", []string{"test", "dir", "sub"}},
		{"/", []string{""}},
		{"/test", []string{"test"}},
		{"/test/dir", []string{"test", "dir"}},
		{"/test/dir/sub", []string{"test", "dir", "sub"}},
	}
	for _, path := range testPaths {
		parts := splitPath(path.path)
		rtest.Equals(t, path.result, parts)
	}
}
