package fs

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/restic/restic/internal/test"
)

// IsRegularFile returns true if fi belongs to a normal file. If fi is nil,
// false is returned.
func IsRegularFile(fi os.FileInfo) bool {
	if fi == nil {
		return false
	}

	return fi.Mode()&(os.ModeType|os.ModeCharDevice) == 0
}

// TestChdir changes the current directory to dest, the function back returns to the previous directory.
func TestChdir(t testing.TB, dest string) (back func()) {
	test.Helper(t).Helper()

	prev, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("chdir to %v", dest)
	err = os.Chdir(dest)
	if err != nil {
		t.Fatal(err)
	}

	return func() {
		test.Helper(t).Helper()
		t.Logf("chdir back to %v", prev)
		err = os.Chdir(prev)
		if err != nil {
			t.Fatal(err)
		}
	}
}

// TestTempFile returns a new temporary file, which is removed when cleanup()
// is called.
func TestTempFile(t testing.TB, prefix string) (File, func()) {
	f, err := ioutil.TempFile("", prefix)
	if err != nil {
		t.Fatal(err)
	}

	cleanup := func() {
		_ = f.Close()
		err = Remove(f.Name())
		if err != nil {
			t.Fatal(err)
		}
	}

	return f, cleanup
}
