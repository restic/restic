// +build !openbsd
// +build !windows

package main

import (
	"fmt"
	"os"

	"restic/fuse"

	systemFuse "bazil.org/fuse"
	"bazil.org/fuse/fs"
)

type CmdMount struct {
	Root bool `long:"owner-root" description:"use 'root' as the owner of files and dirs" default:"false"`

	global *GlobalOptions
	ready  chan struct{}
	done   chan struct{}
}

func init() {
	_, err := parser.AddCommand("mount",
		"mount a repository",
		"The mount command mounts a repository read-only to a given directory",
		&CmdMount{
			global: &globalOpts,
			ready:  make(chan struct{}, 1),
			done:   make(chan struct{}),
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
	root.Add("snapshots", fuse.NewSnapshotsDir(repo, cmd.Root))

	cmd.global.Printf("Now serving %s at %s\n", repo.Backend().Location(), mountpoint)
	cmd.global.Printf("Don't forget to umount after quitting!\n")

	AddCleanupHandler(func() error {
		return systemFuse.Unmount(mountpoint)
	})

	cmd.ready <- struct{}{}

	errServe := make(chan error)
	go func() {
		err = fs.Serve(c, &root)
		if err != nil {
			errServe <- err
		}

		<-c.Ready
		errServe <- c.MountError
	}()

	select {
	case err := <-errServe:
		return err
	case <-cmd.done:
		err := c.Close()
		if err != nil {
			cmd.global.Printf("Error closing fuse connection: %s\n", err)
		}
		return systemFuse.Unmount(mountpoint)
	}
}
