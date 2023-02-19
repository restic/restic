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

	resticfs "github.com/restic/restic/internal/fs"
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

Exit status is 0 if the command was successful, and non-zero if there was any error.
`,
	DisableAutoGenTag: true,
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

	debug.Log("start mount")
	defer debug.Log("finish mount")

	repo, err := OpenRepository(ctx, gopts)
	if err != nil {
		return err
	}

	if !gopts.NoLock {
		var lock *restic.Lock
		lock, ctx, err = lockRepo(ctx, repo)
		defer unlockRepo(lock)
		if err != nil {
			return err
		}
	}

	err = repo.LoadIndex(ctx)
	if err != nil {
		return err
	}

	mountpoint := args[0]

	if _, err := resticfs.Stat(mountpoint); errors.Is(err, os.ErrNotExist) {
		Verbosef("Mountpoint %s doesn't exist\n", mountpoint)
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

	AddCleanupHandler(func(code int) (int, error) {
		debug.Log("running umount cleanup handler for mount at %v", mountpoint)
		err := umount(mountpoint)
		if err != nil {
			Warnf("unable to umount (maybe already umounted or still in use?): %v\n", err)
		}
		// replace error code of sigint
		if code == 130 {
			code = 0
		}
		return code, nil
	})

	c, err := systemFuse.Mount(mountpoint, mountOptions...)
	if err != nil {
		return err
	}

	systemFuse.Debug = func(msg interface{}) {
		debug.Log("fuse: %v", msg)
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
