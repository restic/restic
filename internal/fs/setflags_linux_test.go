package fs

import (
	"io"
	"os"
	"testing"
	"time"

	rtest "github.com/restic/restic/internal/test"

	"golang.org/x/sys/unix"
)

func TestNoatime(t *testing.T) {
	f, err := os.CreateTemp("", "restic-test-noatime")
	if err != nil {
		t.Fatal(err)
	}

	defer func() {
		_ = f.Close()
		err = Remove(f.Name())
		if err != nil {
			t.Fatal(err)
		}
	}()

	// Only run this test on common filesystems that support O_NOATIME.
	// On others, we may not get an error.
	if !supportsNoatime(t, f) {
		t.Skip("temp directory may not support O_NOATIME, skipping")
	}
	// From this point on, we own the file, so we should not get EPERM.

	_, err = io.WriteString(f, "Hello!")
	rtest.OK(t, err)
	_, err = f.Seek(0, io.SeekStart)
	rtest.OK(t, err)

	getAtime := func() time.Time {
		info, err := f.Stat()
		rtest.OK(t, err)
		return ExtendedStat(info).AccessTime
	}

	atime := getAtime()

	err = setFlags(f)
	rtest.OK(t, err)

	_, err = f.Read(make([]byte, 1))
	rtest.OK(t, err)
	rtest.Equals(t, atime, getAtime())
}

func supportsNoatime(t *testing.T, f *os.File) bool {
	var fsinfo unix.Statfs_t
	err := unix.Fstatfs(int(f.Fd()), &fsinfo)
	rtest.OK(t, err)

	// The funky cast works around a compiler error on 32-bit archs:
	// "unix.BTRFS_SUPER_MAGIC (untyped int constant 2435016766) overflows int32".
	// https://github.com/golang/go/issues/52061
	typ := int64(uint(fsinfo.Type))
	return typ == unix.BTRFS_SUPER_MAGIC ||
		typ == unix.EXT2_SUPER_MAGIC ||
		typ == unix.EXT3_SUPER_MAGIC ||
		typ == unix.EXT4_SUPER_MAGIC ||
		typ == unix.TMPFS_MAGIC
}
