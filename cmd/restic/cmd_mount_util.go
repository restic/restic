// +build darwin freebsd linux

package main

import (
	"os"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"

	resticfs "github.com/restic/restic/internal/fs"
	"github.com/restic/restic/internal/fuse"

	systemFuse "bazil.org/fuse"
	"bazil.org/fuse/fs"
)

func mount(opts MountOptions, gopts GlobalOptions, mountpoint string) error {
	debug.Log("start mount")
	defer debug.Log("finish mount")

	repo, err := OpenRepository(gopts)
	if err != nil {
		return err
	}

	lock, err := lockRepo(repo)
	defer unlockRepo(lock)
	if err != nil {
		return err
	}

	err = repo.LoadIndex(gopts.ctx)
	if err != nil {
		return err
	}

	if _, err := resticfs.Stat(mountpoint); os.IsNotExist(errors.Cause(err)) {
		Verbosef("Mountpoint %s doesn't exist, creating it\n", mountpoint)
		err = resticfs.Mkdir(mountpoint, os.ModeDir|0700)
		if err != nil {
			return err
		}
	}

	mountOptions := []systemFuse.MountOption{
		systemFuse.ReadOnly(),
		systemFuse.FSName("restic"),
	}

	if opts.AllowOther {
		mountOptions = append(mountOptions, systemFuse.AllowOther())

		// let the kernel check permissions unless it is explicitly disabled
		if !opts.NoDefaultPermissions {
			mountOptions = append(mountOptions, systemFuse.DefaultPermissions())
		}
	}

	c, err := systemFuse.Mount(mountpoint, mountOptions...)
	if err != nil {
		return err
	}

	systemFuse.Debug = func(msg interface{}) {
		debug.Log("fuse: %v", msg)
	}

	cfg := fuse.Config{
		OwnerIsRoot:      opts.OwnerRoot,
		Hosts:            opts.Hosts,
		Tags:             opts.Tags,
		Paths:            opts.Paths,
		SnapshotTemplate: opts.SnapshotTemplate,
	}
	root := fuse.NewRoot(gopts.ctx, repo, cfg)

	Printf("Now serving the repository at %s\n", mountpoint)
	Printf("When finished, quit with Ctrl-c or umount the mountpoint.\n")

	debug.Log("serving mount at %v", mountpoint)
	err = fs.Serve(c, root)
	if err != nil {
		return err
	}

	<-c.Ready
	return c.MountError
}

func umount(mountpoint string) error {
	return systemFuse.Unmount(mountpoint)
}
