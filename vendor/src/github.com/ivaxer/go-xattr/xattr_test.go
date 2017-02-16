package xattr

import (
	"bytes"
	"io/ioutil"
	"os"
	"sort"
	"testing"
)

var tmpdir = os.Getenv("TEST_XATTR_PATH")

func mktemp(t *testing.T) *os.File {
	file, err := ioutil.TempFile(tmpdir, "test_xattr_")
	if err != nil {
		t.Fatalf("TempFile() failed: %v", err)
	}

	return file
}

func stringsEqual(got, expected []string) bool {
	if len(got) != len(expected) {
		return false
	}

	for i := range got {
		if got[i] != expected[i] {
			return false
		}
	}

	return true
}

// expected must be sorted slice of attribute names.
func checkList(t *testing.T, path string, expected []string) {
	got, err := List(path)
	if err != nil {
		t.Fatalf("List(%q) failed: %v", path, err)
	}

	sort.Strings(got)

	if !stringsEqual(got, expected) {
		t.Errorf("List(%q): expected %v, got %v", path, got, expected)
	}
}

func checkListError(t *testing.T, path string, f func(error) bool) {
	got, err := List(path)
	if !f(err) {
		t.Errorf("List(%q): unexpected error value: %v", path, err)
	}

	if got != nil {
		t.Error("List(): expected nil slice on error")
	}
}

func checkSet(t *testing.T, path, attr string, data []byte) {
	if err := Set(path, attr, data); err != nil {
		t.Fatalf("Set(%q, %q, %v) failed: %v", path, attr, data, err)
	}
}

func checkSetError(t *testing.T, path, attr string, data []byte, f func(error) bool) {
	if err := Set(path, attr, data); !f(err) {
		t.Fatalf("Set(%q, %q, %v): unexpected error value: %v", path, attr, data, err)
	}
}

func checkGet(t *testing.T, path, attr string, expected []byte) {
	got, err := Get(path, attr)
	if err != nil {
		t.Fatalf("Get(%q, %q) failed: %v", path, attr, err)
	}

	if !bytes.Equal(got, expected) {
		t.Errorf("Get(%q, %q): got %v, expected %v", path, attr, got, expected)
	}
}

func checkGetError(t *testing.T, path, attr string, f func(error) bool) {
	got, err := Get(path, attr)
	if !f(err) {
		t.Errorf("Get(%q, %q): unexpected error value: %v", path, attr, err)
	}

	if got != nil {
		t.Error("Get(): expected nil slice on error")
	}
}

func checkRemove(t *testing.T, path, attr string) {
	if err := Remove(path, attr); err != nil {
		t.Fatalf("Remove(%q, %q) failed: %v", path, attr, err)
	}
}

func checkRemoveError(t *testing.T, path, attr string, f func(error) bool) {
	if err := Remove(path, attr); !f(err) {
		t.Errorf("Remove(%q, %q): unexpected error value: %v", path, attr, err)
	}
}

func TestFlow(t *testing.T) {
	f := mktemp(t)
	defer func() { f.Close(); os.Remove(f.Name()) }()

	path := f.Name()
	data := []byte("test xattr data")
	attr := "user.test xattr"
	attr2 := "user.text xattr 2"

	checkList(t, path, []string{})
	checkSet(t, path, attr, data)
	checkList(t, path, []string{attr})
	checkSet(t, path, attr2, data)
	checkList(t, path, []string{attr, attr2})
	checkGet(t, path, attr, data)
	checkGetError(t, path, "user.unknown attr", IsNotExist)
	checkRemove(t, path, attr)
	checkList(t, path, []string{attr2})
	checkRemove(t, path, attr2)
	checkList(t, path, []string{})
}

func TestEmptyAttr(t *testing.T) {
	f := mktemp(t)
	defer func() { f.Close(); os.Remove(f.Name()) }()

	path := f.Name()
	attr := "user.test xattr"
	data := []byte{}

	checkSet(t, path, attr, data)
	checkList(t, path, []string{attr})
	checkGet(t, path, attr, []byte{})
	checkRemove(t, path, attr)
	checkList(t, path, []string{})
}

func TestNoFile(t *testing.T) {
	path := "no-such-file"
	attr := "user.test xattr"
	data := []byte("test_xattr data")

	checkListError(t, path, os.IsNotExist)
	checkSetError(t, path, attr, data, os.IsNotExist)
	checkGetError(t, path, attr, os.IsNotExist)
	checkRemoveError(t, path, attr, os.IsNotExist)
}
