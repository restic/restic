//go:build darwin || freebsd || linux
// +build darwin freebsd linux

package main

import (
	"context"
	"fmt"
	"os"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/global"
	"github.com/restic/restic/internal/ui"

	"github.com/restic/restic/internal/fuse"

	systemFuse "github.com/anacrolix/fuse"
	"github.com/anacrolix/fuse/fs"
)

func runMount(ctx context.Context, opts MountOptions, gopts global.Options, args []string, term ui.Terminal) error {
	err := runMountCheck(ctx, opts, gopts, args, term)
	if err != nil {
		return err
	}

	printer := ui.NewProgressPrinter(false, gopts.Verbosity, term)

	if len(args) == 0 {
		return errors.Fatal("wrong number of parameters")
	}

	mountpoint := args[0]

	// Check the existence of the mount point at the earliest stage to
	// prevent unnecessary computations while opening the repository.
	if _, err := os.Stat(mountpoint); errors.Is(err, os.ErrNotExist) {
		printer.P("Mountpoint %s doesn't exist", mountpoint)
		return err
	}

	debug.Log("start mount")
	defer debug.Log("finish mount")

	ctx, repo, unlock, err := openWithReadLock(ctx, gopts, gopts.NoLock, printer)
	if err != nil {
		return err
	}
	defer unlock()

	err = repo.LoadIndex(ctx, printer)
	if err != nil {
		return err
	}

	fuseMountName := fmt.Sprintf("restic:%s", repo.Config().ID[:10])

	mountOptions := []systemFuse.MountOption{
		systemFuse.ReadOnly(),
		systemFuse.FSName(fuseMountName),
		systemFuse.MaxReadahead(128 * 1024),
	}

	if opts.AllowOther {
		mountOptions = append(mountOptions, systemFuse.AllowOther())

		// let the kernel check permissions unless it is explicitly disabled
		if !opts.NoDefaultPermissions {
			mountOptions = append(mountOptions, systemFuse.DefaultPermissions())
		}
	}

	systemFuse.Debug = func(msg interface{}) {
		debug.Log("fuse: %v", msg)
	}

	c, err := systemFuse.Mount(mountpoint, mountOptions...)
	if err != nil {
		return err
	}

	cfg := fuse.Config{
		OwnerIsRoot:   opts.OwnerRoot,
		Filter:        opts.SnapshotFilter,
		TimeTemplate:  opts.TimeTemplate,
		PathTemplates: opts.PathTemplates,
	}
	root := fuse.NewRoot(repo, cfg)
	fuseFS := fuse.NewFS(root)

	printer.S("Now serving the repository at %s", mountpoint)
	printer.S("Use another terminal or tool to browse the contents of this folder.")
	printer.S("When finished, quit with Ctrl-c here or umount the mountpoint.")

	debug.Log("serving mount at %v", mountpoint)

	done := make(chan struct{})

	go func() {
		defer close(done)
		err = fs.Serve(c, fuseFS)
	}()

	select {
	case <-ctx.Done():
		debug.Log("running umount cleanup handler for mount at %v", mountpoint)
		err := systemFuse.Unmount(mountpoint)
		if err != nil {
			printer.E("unable to umount (maybe already umounted or still in use?): %v", err)
		}

		return ErrOK
	case <-done:
		// clean shutdown, nothing to do
	}

	return err
}
