//go:build darwin || freebsd || linux
// +build darwin freebsd linux

package main

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/restic"

	"github.com/restic/restic/internal/fuse"

	systemFuse "github.com/anacrolix/fuse"
	"github.com/anacrolix/fuse/fs"
)

var cmdMount = &cobra.Command{
	Use:   "mount [flags] mountpoint",
	Short: "Mount the repository",
	Long: `
The "mount" command mounts the repository via fuse to a directory. This is a
read-only mount.

Snapshot Directories
====================

If you need a different template for directories that contain snapshots,
you can pass a time template via --time-template and path templates via
--path-template.

Example time template without colons:

    --time-template "2006-01-02_15-04-05"

You need to specify a sample format for exactly the following timestamp:

    Mon Jan 2 15:04:05 -0700 MST 2006

For details please see the documentation for time.Format() at:
  https://godoc.org/time#Time.Format

For path templates, you can use the following patterns which will be replaced:
    %i by short snapshot ID
    %I by long snapshot ID
    %u by username
    %h by hostname
    %t by tags
    %T by timestamp as specified by --time-template

The default path templates are:
    "ids/%i"
    "snapshots/%T"
    "hosts/%h/%T"
    "tags/%t/%T"

EXIT STATUS
===========

Exit status is 0 if the command was successful.
Exit status is 1 if there was any error.
Exit status is 10 if the repository does not exist.
Exit status is 11 if the repository is already locked.
Exit status is 12 if the password is incorrect.
`,
	DisableAutoGenTag: true,
	GroupID:           cmdGroupDefault,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runMount(cmd.Context(), mountOptions, globalOptions, args)
	},
}

// MountOptions collects all options for the mount command.
type MountOptions struct {
	OwnerRoot            bool
	AllowOther           bool
	NoDefaultPermissions bool
	restic.SnapshotFilter
	TimeTemplate  string
	PathTemplates []string
}

var mountOptions MountOptions

func init() {
	cmdRoot.AddCommand(cmdMount)

	mountFlags := cmdMount.Flags()
	mountFlags.BoolVar(&mountOptions.OwnerRoot, "owner-root", false, "use 'root' as the owner of files and dirs")
	mountFlags.BoolVar(&mountOptions.AllowOther, "allow-other", false, "allow other users to access the data in the mounted directory")
	mountFlags.BoolVar(&mountOptions.NoDefaultPermissions, "no-default-permissions", false, "for 'allow-other', ignore Unix permissions and allow users to read all snapshot files")

	initMultiSnapshotFilter(mountFlags, &mountOptions.SnapshotFilter, true)

	mountFlags.StringArrayVar(&mountOptions.PathTemplates, "path-template", nil, "set `template` for path names (can be specified multiple times)")
	mountFlags.StringVar(&mountOptions.TimeTemplate, "snapshot-template", time.RFC3339, "set `template` to use for snapshot dirs")
	mountFlags.StringVar(&mountOptions.TimeTemplate, "time-template", time.RFC3339, "set `template` to use for times")
	_ = mountFlags.MarkDeprecated("snapshot-template", "use --time-template")
}

func runMount(ctx context.Context, opts MountOptions, gopts GlobalOptions, args []string) error {
	if opts.TimeTemplate == "" {
		return errors.Fatal("time template string cannot be empty")
	}

	if strings.HasPrefix(opts.TimeTemplate, "/") || strings.HasSuffix(opts.TimeTemplate, "/") {
		return errors.Fatal("time template string cannot start or end with '/'")
	}

	if len(args) == 0 {
		return errors.Fatal("wrong number of parameters")
	}

	mountpoint := args[0]

	// Check the existence of the mount point at the earliest stage to
	// prevent unnecessary computations while opening the repository.
	if _, err := os.Stat(mountpoint); errors.Is(err, os.ErrNotExist) {
		Verbosef("Mountpoint %s doesn't exist\n", mountpoint)
		return err
	}

	debug.Log("start mount")
	defer debug.Log("finish mount")

	ctx, repo, unlock, err := openWithReadLock(ctx, gopts, gopts.NoLock)
	if err != nil {
		return err
	}
	defer unlock()

	bar := newIndexProgress(gopts.Quiet, gopts.JSON)
	err = repo.LoadIndex(ctx, bar)
	if err != nil {
		return err
	}

	mountOptions := []systemFuse.MountOption{
		systemFuse.ReadOnly(),
		systemFuse.FSName("restic"),
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

	Printf("Now serving the repository at %s\n", mountpoint)
	Printf("Use another terminal or tool to browse the contents of this folder.\n")
	Printf("When finished, quit with Ctrl-c here or umount the mountpoint.\n")

	debug.Log("serving mount at %v", mountpoint)

	done := make(chan struct{})

	go func() {
		defer close(done)
		err = fs.Serve(c, root)
	}()

	select {
	case <-ctx.Done():
		debug.Log("running umount cleanup handler for mount at %v", mountpoint)
		err := systemFuse.Unmount(mountpoint)
		if err != nil {
			Warnf("unable to umount (maybe already umounted or still in use?): %v\n", err)
		}

		return ErrOK
	case <-done:
		// clean shutdown, nothing to do
	}

	return err
}
