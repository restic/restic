//go:build linux

package archiver

import (
	"os"
	"testing"

	"golang.org/x/sys/unix"

	"github.com/restic/restic/internal/test"
)

func TestRejectByNoDump(t *testing.T) {
	tempDir := test.TempDir(t)

	items := []struct {
		path   string
		dir    bool
		noDump bool
	}{
		{"/no-dump", true, true},
		{"/normal", true, false},
		{"/normal/no-dump", false, true},
		{"/normal/normal", false, false},
	}

	for _, item := range items {
		if item.dir {
			test.OK(t, os.Mkdir(tempDir+item.path, 0700))
		} else {
			test.OK(t, os.WriteFile(tempDir+item.path, nil, 0600))
		}

		if item.noDump {
			test.OK(t, setNoDump(tempDir+item.path))
		}
	}

	reject, err := RejectByNoDump(nil)
	test.OK(t, err)

	for _, item := range items {
		rejected := reject(tempDir+item.path, nil, nil)
		if rejected != item.noDump {
			t.Errorf("inclusion status of %s is wrong: want %v, got %v", item.path, item.noDump, rejected)
		}
	}
}

// setNoDump sets the "no dump" Linux file attribute on path (file or directory).
func setNoDump(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	return unix.IoctlSetPointerInt(int(f.Fd()), unix.FS_IOC_SETFLAGS, 0x40 /* FS_NODUMP_FL */)
}
