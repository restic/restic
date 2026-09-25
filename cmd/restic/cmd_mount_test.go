//go:build darwin || freebsd || linux

package main

import (
	"runtime"
	"testing"

	"github.com/restic/restic/internal/global"
	rtest "github.com/restic/restic/internal/test"
)

func TestFileManagerCmd(t *testing.T) {
	cmd := fileManagerCmd("/mnt/repo")

	var wantBin string
	switch runtime.GOOS {
	case "darwin":
		wantBin = "open"
	default:
		wantBin = "xdg-open"
	}

	rtest.Equals(t, []string{wantBin, "/mnt/repo"}, cmd.Args)
}

func TestOpenFileManagerFlag(t *testing.T) {
	gopts := global.Options{}
	cmd := newMountCommand(&gopts)
	f := cmd.Flags().Lookup("open-file-manager")
	rtest.Assert(t, f != nil, "flag --open-file-manager must be registered")
	rtest.Equals(t, "false", f.DefValue)
}
