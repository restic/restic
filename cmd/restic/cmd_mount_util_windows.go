package main

import (
	windowsFuse "github.com/billziss-gh/cgofuse/fuse"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/fuse"
)

func mount(opts MountOptions, gopts GlobalOptions, mountpoint string) (result error) {
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

	cfg := fuse.Config{
		OwnerIsRoot:      opts.OwnerRoot,
		Hosts:            opts.Hosts,
		Tags:             opts.Tags,
		Paths:            opts.Paths,
		SnapshotTemplate: opts.SnapshotTemplate,
	}
	fuseFsWindows := fuse.NewFuseFsWindows(gopts.ctx, repo, cfg)

	host := windowsFuse.NewFileSystemHost(fuseFsWindows)
	host.SetCapReaddirPlus(true)

	// Right now it is not possible to check if winfsp is installed before
	// mounting the filesystem. Until https://github.com/billziss-gh/cgofuse/issues/48
	// is closed this is a workaround to handle it in a user-friendly way.
	defer func() {
		if r := recover(); r != nil {
			result = errors.Fatalf(
				"mount failed, please make sure winfsp is installed: %v", r,
			)
		}
	}()

	success := host.Mount(mountpoint, []string{})

	if !success {
		return errors.Fatal("mount failed")
	}

	return nil
}

func umount(mountpoint string) error {
	// TODO: call host.Umount()
	return nil
}
