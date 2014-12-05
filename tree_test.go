package restic_test

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
)

var testFiles = []struct {
	name    string
	content []byte
}{
	{"foo", []byte("bar")},
	{"bar/foo2", []byte("bar2")},
	{"bar/bla/blubb", []byte("This is just a test!\n")},
}

// prepareDir creates a temporary directory and returns it.
func prepareDir(t *testing.T) string {
	tempdir, err := ioutil.TempDir("", "restic-test-")
	ok(t, err)

	for _, test := range testFiles {
		file := filepath.Join(tempdir, test.name)
		dir := filepath.Dir(file)
		if dir != "." {
			ok(t, os.MkdirAll(dir, 0755))
		}

		f, err := os.Create(file)
		defer func() {
			ok(t, f.Close())
		}()

		ok(t, err)

		_, err = f.Write(test.content)
		ok(t, err)
	}

	return tempdir
}

func TestTree(t *testing.T) {
	dir := prepareDir(t)
	defer func() {
		if *testCleanup {
			ok(t, os.RemoveAll(dir))
		}
	}()
}
