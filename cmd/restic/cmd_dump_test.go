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

func TestDumpPreparePathList(t *testing.T) {
	testPaths := []struct {
		paths  []string
		result [][]string
	}{
		{
			[]string{"test", "test/dir", "test/dir/sub"},
			[][]string{{"test"}},
		},
		{
			[]string{"/", "man", "doc", "doc/icons"},
			[][]string{{""}},
		},
		{
			[]string{"doc"},
			[][]string{{"doc"}},
		},
		{
			[]string{"man/", "doc", "doc/icons"},
			[][]string{{"man"}, {"doc"}},
		},
	}
	for _, path := range testPaths {
		parts := preparePathList(path.paths)
		rtest.Equals(t, path.result, parts)
	}
}
