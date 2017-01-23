package backend_test

import (
	"restic"
	"testing"

	"restic/backend"
)

var pathsTests = []struct {
	base      string
	t         restic.FileType
	name      string
	prefixLen int
	res       string
}{
	{"base", restic.DataFile, "abcdef", 0, "base/data/abcdef"},
	{"base", restic.DataFile, "abcdef", 2, "base/data/ab/abcdef"},
	{"base", restic.DataFile, "abcdef", 4, "base/data/abcd/abcdef"},
	{"base", restic.DataFile, "ab", 2, "base/data/ab"},
	{"base", restic.DataFile, "abcd", 4, "base/data/abcd"},

	{"", restic.DataFile, "abcdef", 0, "data/abcdef"},
	{"", restic.DataFile, "abcdef", 2, "data/ab/abcdef"},
	{"", restic.DataFile, "abcdef", 4, "data/abcd/abcdef"},
	{"", restic.DataFile, "ab", 2, "data/ab"},
	{"", restic.DataFile, "abcd", 4, "data/abcd"},

	{"base", restic.ConfigFile, "file", 0, "base/config"},
	{"", restic.ConfigFile, "file", 0, "config"},

	{"base", restic.SnapshotFile, "file", 0, "base/snapshots/file"},
	{"base", restic.SnapshotFile, "file", 2, "base/snapshots/file"},

	{"base", restic.IndexFile, "file", 0, "base/index/file"},
	{"base", restic.IndexFile, "file", 2, "base/index/file"},

	{"base", restic.LockFile, "file", 0, "base/locks/file"},
	{"base", restic.LockFile, "file", 2, "base/locks/file"},

	{"base", restic.KeyFile, "file", 0, "base/keys/file"},
	{"base", restic.KeyFile, "file", 2, "base/keys/file"},
}

func TestFilename(t *testing.T) {
	for i, test := range pathsTests {
		res := backend.Filename(test.base, test.t, test.name, test.prefixLen)

		if test.res != res {
			t.Errorf("test %d: result does not match, want %q, got %q",
				i, test.res, res)
		}
	}

}
