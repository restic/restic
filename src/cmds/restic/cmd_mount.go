// +build !openbsd
// +build !windows

package main

import (
	"os"

	"github.com/spf13/cobra"

	"restic/debug"
	"restic/errors"

	resticfs "restic/fs"
	"restic/fuse"

	systemFuse "bazil.org/fuse"
	"bazil.org/fuse/fs"
)

var cmdMount = &cobra.Command{
	Use:   "mount [flags] mountpoint",
	Short: "mount the repository",
	Long: `
The "mount" command mounts the repository via fuse to a directory. This is a
read-only mount.
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runMount(mountOptions, globalOptions, args)
	},
}

// MountOptions collects all options for the mount command.
type MountOptions struct {
	OwnerRoot bool
}

var mountOptions MountOptions

func init() {
	cmdRoot.AddCommand(cmdMount)

	cmdMount.Flags().BoolVar(&mountOptions.OwnerRoot, "owner-root", false, "use 'root' as the owner of files and dirs")
}

func mount(opts MountOptions, gopts GlobalOptions, mountpoint string) error {
	debug.Log("start mount")
	defer debug.Log("finish mount")

	repo, err := OpenRepository(gopts)
	if err != nil {
		return err
	}

	err = repo.LoadIndex()
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
	c, err := systemFuse.Mount(
		mountpoint,
		systemFuse.ReadOnly(),
		systemFuse.FSName("restic"),
	)
	if err != nil {
		return err
	}

	Printf("Now serving the repository at %s\n", mountpoint)
	Printf("Don't forget to umount after quitting!\n")

	root := fs.Tree{}
	root.Add("snapshots", fuse.NewSnapshotsDir(repo, opts.OwnerRoot))

	debug.Log("serving mount at %v", mountpoint)
	err = fs.Serve(c, &root)
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
	if len(args) == 0 {
		return errors.Fatalf("wrong number of parameters")
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
