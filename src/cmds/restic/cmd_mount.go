// +build !openbsd
// +build !windows

package main

import (
	"os"

	"restic/errors"

	resticfs "restic/fs"
	"restic/fuse"

	systemFuse "bazil.org/fuse"
	"bazil.org/fuse/fs"
)

type CmdMount struct {
	Root bool `long:"owner-root" description:"use 'root' as the owner of files and dirs" default:"false"`

	global *GlobalOptions
	ready  chan struct{}
}

func init() {
	_, err := parser.AddCommand("mount",
		"mount a repository",
		"The mount command mounts a repository read-only to a given directory",
		&CmdMount{
			global: &globalOpts,
			ready:  make(chan struct{}),
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
		return errors.Fatalf("wrong number of parameters, Usage: %s", cmd.Usage())
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
	if _, err := resticfs.Stat(mountpoint); os.IsNotExist(errors.Cause(err)) {
		cmd.global.Verbosef("Mountpoint %s doesn't exist, creating it\n", mountpoint)
		err = resticfs.Mkdir(mountpoint, os.ModeDir|0700)
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
	root.Add("snapshots", fuse.NewSnapshotsDir(repo, cmd.Root))

	cmd.global.Printf("Now serving %s at %s\n", repo.Backend().Location(), mountpoint)
	cmd.global.Printf("Don't forget to umount after quitting!\n")

	AddCleanupHandler(func() error {
		err := systemFuse.Unmount(mountpoint)
		if err != nil {
			cmd.global.Warnf("unable to umount (maybe already umounted?): %v\n", err)
		}
		return nil
	})

	close(cmd.ready)

	err = fs.Serve(c, &root)
	if err != nil {
		return err
	}

	<-c.Ready
	return c.MountError
}
