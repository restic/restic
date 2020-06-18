package rclone

import (
	"context"
	"testing"

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
	rtest.OK(t, err)
	defer be.Close()

	err = be.cmd.Process.Kill()
	rtest.OK(t, err)
	t.Log("killed rclone")

	_, err = be.Stat(context.TODO(), restic.Handle{
		Name: "foo",
		Type: restic.DataFile,
	})
	rtest.Assert(t, err != nil, "expected an error")
}
