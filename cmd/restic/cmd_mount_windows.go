//go:build windows
// +build windows

package main

import (
	"context"
	"os"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/global"
	"github.com/restic/restic/internal/ui"

	"github.com/restic/restic/internal/fuse"

	systemFuse "github.com/winfsp/cgofuse/fuse"
)

func runMount(ctx context.Context, opts MountOptions, gopts global.Options, args []string, term ui.Terminal) error {
	err := runMountCheck(ctx, opts, gopts, args, term)
	if err != nil {
		return err
	}

	printer := ui.NewProgressPrinter(false, gopts.Verbosity, term)

	//allow empty mount point
	mountpoint := ""
	if len(args) == 1 {
		mountpoint = args[0]
		// Check the existence of the mount point at the earliest stage to
		// prevent unnecessary computations while opening the repository.
		if _, err := os.Stat(mountpoint); errors.Is(err, os.ErrNotExist) {
			printer.P("Mountpoint %s doesn't exist", mountpoint)
			return err
		}
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

	cfg := fuse.Config{
		OwnerIsRoot:   opts.OwnerRoot,
		Filter:        opts.SnapshotFilter,
		TimeTemplate:  opts.TimeTemplate,
		PathTemplates: opts.PathTemplates,
	}
	root := fuse.NewRoot(repo, cfg)
	windowsFS := fuse.NewWindowsFS(ctx, root) // Pass the runMount context
	windowsFSCanceller, ok := windowsFS.(fuse.WindowsFSCanceller)
	if !ok {
		return errors.Fatal("failed to cast windowsFS to fuse.WindowsFSCanceller")
	}

	host := systemFuse.NewFileSystemHost(windowsFS)
	host.SetCapReaddirPlus(true)
	host.SetUseIno(true) // FUSE3 only

	printer.S("Now serving the repository at %s", mountpoint)
	printer.S("Use another terminal or tool to browse the contents of this folder.")
	printer.S("When finished, quit with Ctrl-c here or umount the mountpoint.")

	debug.Log("serving mount at %v", mountpoint)

	// cgofuse's Mount blocks until unmounted or error
	// It takes a slice of strings for options, similar to os.Args
	// For now, we'll pass an empty slice, or just the mountpoint
	mountArgs := []string{mountpoint}
	if opts.AllowOther {
		// cgofuse equivalent of allow-other might be different or not directly supported
		// For now, we'll omit specific options unless cgofuse docs clarify
	}

	done := make(chan struct{})

	go func() {
		defer close(done)
		// The cgofuse Mount function returns a bool indicating success or failure.
		// Errors are typically logged by cgofuse internally.
		success := host.Mount(mountpoint, mountArgs)
		if !success {
			err = errors.Fatal("cgofuse mount failed")
		}
	}()

	select {
	case <-ctx.Done():
		<-ctx.Done()
		debug.Log("context cancelled, attempting to unmount %s", mountpoint)
		// Call the cancel function of the windowsFSBridge to signal its operations to stop
		windowsFSCanceller.Cancel()
		// cgofuse's Unmount function can be called from a separate goroutine.
		// It returns true on success, false otherwise.
		if host.Unmount() {
			debug.Log("successfully unmounted %s", mountpoint)
		} else {
			debug.Log("failed to unmount %s", mountpoint)
		}
	case <-done:
		// clean shutdown, nothing to do
	}

	return err
}
