package khepri_test

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

// prepare directory and return temporary path
func prepare_dir(t *testing.T) string {
	tempdir, err := ioutil.TempDir("", "khepri-test-")
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

	t.Logf("tempdir prepared at %s", tempdir)

	return tempdir
}

func TestTree(t *testing.T) {
	dir := prepare_dir(t)
	defer func() {
		if *testCleanup {
			ok(t, os.RemoveAll(dir))
		}
	}()
}
