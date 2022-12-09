package restorer

import (
	"os"
	"path"
	"strconv"
	"syscall"
	"testing"

	"github.com/restic/restic/internal/fs"
	"github.com/restic/restic/internal/test"
)

func TestPreallocate(t *testing.T) {
	for _, i := range []int64{0, 1, 4096, 1024 * 1024} {
		t.Run(strconv.FormatInt(i, 10), func(t *testing.T) {
			dirpath := test.TempDir(t)

			flags := os.O_CREATE | os.O_TRUNC | os.O_WRONLY
			wr, err := os.OpenFile(path.Join(dirpath, "test"), flags, 0600)
			test.OK(t, err)
			defer func() {
				test.OK(t, wr.Close())
			}()

			err = preallocateFile(wr, i)
			if err == syscall.ENOTSUP {
				t.SkipNow()
			}
			test.OK(t, err)

			fi, err := wr.Stat()
			test.OK(t, err)

			efi := fs.ExtendedStat(fi)
			test.Assert(t, efi.Size == i || efi.Blocks > 0, "Preallocated size of %v, got size %v block %v", i, efi.Size, efi.Blocks)
		})
	}
}
