package fs

import (
	"io/ioutil"
	"os"
	"testing"
)

// IsRegularFile returns true if fi belongs to a normal file. If fi is nil,
// false is returned.
func IsRegularFile(fi os.FileInfo) bool {
	if fi == nil {
		return false
	}

	return fi.Mode()&(os.ModeType|os.ModeCharDevice) == 0
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
