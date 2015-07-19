// +build !openbsd

package main

import (
	"fmt"
	"os"

	"github.com/restic/restic/cmd/restic/fuse"

	systemFuse "bazil.org/fuse"
	"bazil.org/fuse/fs"
)

type CmdMount struct {
	global *GlobalOptions
	ready  chan struct{}
}

func init() {
	_, err := parser.AddCommand("mount",
		"mount a repository",
		"The mount command mounts a repository read-only to a given directory",
		&CmdMount{
			global: &globalOpts,
			ready:  make(chan struct{}, 1),
		})
	if err != nil {
		panic(err)
	}
}

func (cmd CmdMount) Usage() string {
	return "MOUNTPOINT"
}

func (cmd CmdMount) Execute(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("wrong number of parameters, Usage: %s", cmd.Usage())
	}

	repo, err := cmd.global.OpenRepository()
	if err != nil {
		return err
	}

	err = repo.LoadIndex()
	if err != nil {
		return err
	}

	mountpoint := args[0]
	if _, err := os.Stat(mountpoint); os.IsNotExist(err) {
		cmd.global.Verbosef("Mountpoint %s doesn't exist, creating it\n", mountpoint)
		err = os.Mkdir(mountpoint, os.ModeDir|0700)
		if err != nil {
			return err
		}
	}
	c, err := systemFuse.Mount(
		mountpoint,
		systemFuse.ReadOnly(),
		systemFuse.FSName("restic"),
	)
	if err != nil {
		return err
	}

	root := fs.Tree{}
	root.Add("snapshots", fuse.NewSnapshotsDir(repo))

	cmd.global.Printf("Now serving %s at %s\n", repo.Backend().Location(), mountpoint)
	cmd.global.Printf("Don't forget to umount after quitting!\n")

	cmd.ready <- struct{}{}

	err = fs.Serve(c, &root)
	if err != nil {
		return err
	}

	<-c.Ready
	return c.MountError
}
