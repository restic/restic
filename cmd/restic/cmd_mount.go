// +build !netbsd
// +build !openbsd
// +build !solaris
// +build !windows

package main

import (
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/restic"

	resticfs "github.com/restic/restic/internal/fs"
	"github.com/restic/restic/internal/fuse"

	systemFuse "bazil.org/fuse"
	"bazil.org/fuse/fs"
)

var cmdMount = &cobra.Command{
	Use:   "mount [flags] mountpoint",
	Short: "Mount the repository",
	Long: `
The "mount" command mounts the repository via fuse to a directory. This is a
read-only mount.

Snapshot Directories
====================

If you need a different template for all directories that contain snapshots,
you can pass a template via --snapshot-template. Example without colons:

    --snapshot-template "2006-01-02_15-04-05"

You need to specify a sample format for exactly the following timestamp:

    Mon Jan 2 15:04:05 -0700 MST 2006

For details please see the documentation for time.Format() at:
  https://godoc.org/time#Time.Format
`,
	DisableAutoGenTag: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runMount(mountOptions, globalOptions, args)
	},
}

// MountOptions collects all options for the mount command.
type MountOptions struct {
	OwnerRoot            bool
	AllowRoot            bool
	AllowOther           bool
	NoDefaultPermissions bool
	Host                 string
	Tags                 restic.TagLists
	Paths                []string
	SnapshotTemplate     string
}

var mountOptions MountOptions

func init() {
	cmdRoot.AddCommand(cmdMount)

	mountFlags := cmdMount.Flags()
	mountFlags.BoolVar(&mountOptions.OwnerRoot, "owner-root", false, "use 'root' as the owner of files and dirs")
	mountFlags.BoolVar(&mountOptions.AllowRoot, "allow-root", false, "allow root user to access the data in the mounted directory")
	mountFlags.BoolVar(&mountOptions.AllowOther, "allow-other", false, "allow other users to access the data in the mounted directory")
	mountFlags.BoolVar(&mountOptions.NoDefaultPermissions, "no-default-permissions", false, "for 'allow-other', ignore Unix permissions and allow users to read all snapshot files")

	mountFlags.StringVarP(&mountOptions.Host, "host", "H", "", `only consider snapshots for this host`)
	mountFlags.Var(&mountOptions.Tags, "tag", "only consider snapshots which include this `taglist`")
	mountFlags.StringArrayVar(&mountOptions.Paths, "path", nil, "only consider snapshots which include this (absolute) `path`")

	mountFlags.StringVar(&mountOptions.SnapshotTemplate, "snapshot-template", time.RFC3339, "set `template` to use for snapshot dirs")
}

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

	if opts.AllowRoot {
		mountOptions = append(mountOptions, systemFuse.AllowRoot())
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
		Host:             opts.Host,
		Tags:             opts.Tags,
		Paths:            opts.Paths,
		SnapshotTemplate: opts.SnapshotTemplate,
	}
	root, err := fuse.NewRoot(gopts.ctx, repo, cfg)
	if err != nil {
		return err
	}

	Printf("Now serving the repository at %s\n", mountpoint)
	Printf("Don't forget to umount after quitting!\n")

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

func runMount(opts MountOptions, gopts GlobalOptions, args []string) error {
	if opts.SnapshotTemplate == "" {
		return errors.Fatal("snapshot template string cannot be empty")
	}

	if strings.ContainsAny(opts.SnapshotTemplate, `\/`) {
		return errors.Fatal("snapshot template string contains a slash (/) or backslash (\\) character")
	}

	if len(args) == 0 {
		return errors.Fatal("wrong number of parameters")
	}

	mountpoint := args[0]

	AddCleanupHandler(func() error {
		debug.Log("running umount cleanup handler for mount at %v", mountpoint)
		err := umount(mountpoint)
		if err != nil {
			Warnf("unable to umount (maybe already umounted?): %v\n", err)
		}
		return nil
	})

	return mount(opts, gopts, mountpoint)
}
