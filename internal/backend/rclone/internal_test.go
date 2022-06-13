package rclone

import (
	"context"
	"os/exec"
	"testing"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
)

// restic should detect rclone exiting.
func TestRcloneExit(t *testing.T) {
	dir, cleanup := rtest.TempDir(t)
	defer cleanup()

	cfg := NewConfig()
	cfg.Remote = dir
	be, err := Open(cfg, nil)
	var e *exec.Error
	if errors.As(err, &e) && e.Err == exec.ErrNotFound {
		t.Skipf("program %q not found", e.Name)
		return
	}
	rtest.OK(t, err)
	defer func() {
		// ignore the error as the test will kill rclone (see below)
		_ = be.Close()
	}()

	err = be.cmd.Process.Kill()
	rtest.OK(t, err)
	t.Log("killed rclone")

	for i := 0; i < 10; i++ {
		_, err = be.Stat(context.TODO(), restic.Handle{
			Name: "foo",
			Type: restic.PackFile,
		})
		rtest.Assert(t, err != nil, "expected an error")
	}
}
