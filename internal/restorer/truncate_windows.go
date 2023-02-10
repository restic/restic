package restorer

import (
	"os"

	"github.com/restic/restic/internal/debug"
	"golang.org/x/sys/windows"
)

func truncateSparse(f *os.File, size int64) error {
	// try setting the sparse file attribute, but ignore the error if it fails
	var t uint32
	err := windows.DeviceIoControl(windows.Handle(f.Fd()), windows.FSCTL_SET_SPARSE, nil, 0, nil, 0, &t, nil)
	if err != nil {
		debug.Log("failed to set sparse attribute for %v: %v", f.Name(), err)
	}

	return f.Truncate(size)
}
