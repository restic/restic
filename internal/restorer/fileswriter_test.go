package restorer

import (
	"os"
	"testing"

	rtest "github.com/restic/restic/internal/test"
)

func TestFilesWriterBasic(t *testing.T) {
	dir := rtest.TempDir(t)
	w := newFilesWriter(1)

	f1 := dir + "/f1"
	f2 := dir + "/f2"

	rtest.OK(t, w.writeToFile(f1, []byte{1}, 0, 2, false))
	rtest.Equals(t, 0, len(w.buckets[0].files))

	rtest.OK(t, w.writeToFile(f2, []byte{2}, 0, 2, false))
	rtest.Equals(t, 0, len(w.buckets[0].files))

	rtest.OK(t, w.writeToFile(f1, []byte{1}, 1, -1, false))
	rtest.Equals(t, 0, len(w.buckets[0].files))

	rtest.OK(t, w.writeToFile(f2, []byte{2}, 1, -1, false))
	rtest.Equals(t, 0, len(w.buckets[0].files))

	buf, err := os.ReadFile(f1)
	rtest.OK(t, err)
	rtest.Equals(t, []byte{1, 1}, buf)

	buf, err = os.ReadFile(f2)
	rtest.OK(t, err)
	rtest.Equals(t, []byte{2, 2}, buf)
}
