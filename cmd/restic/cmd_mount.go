// +build darwin freebsd linux windows

package main

import (
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/restic"
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

EXIT STATUS
===========

Exit status is 0 if the command was successful, and non-zero if there was any error.
`,
	DisableAutoGenTag: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runMount(mountOptions, globalOptions, args)
	},
}

// MountOptions collects all options for the mount command.
type MountOptions struct {
	OwnerRoot            bool
	AllowOther           bool
	NoDefaultPermissions bool
	Hosts                []string
	Tags                 restic.TagLists
	Paths                []string
	SnapshotTemplate     string
}

var mountOptions MountOptions

func init() {
	cmdRoot.AddCommand(cmdMount)

	mountFlags := cmdMount.Flags()
	mountFlags.BoolVar(&mountOptions.OwnerRoot, "owner-root", false, "use 'root' as the owner of files and dirs")
	mountFlags.BoolVar(&mountOptions.AllowOther, "allow-other", false, "allow other users to access the data in the mounted directory")
	mountFlags.BoolVar(&mountOptions.NoDefaultPermissions, "no-default-permissions", false, "for 'allow-other', ignore Unix permissions and allow users to read all snapshot files")

	mountFlags.StringArrayVarP(&mountOptions.Hosts, "host", "H", nil, `only consider snapshots for this host (can be specified multiple times)`)
	mountFlags.Var(&mountOptions.Tags, "tag", "only consider snapshots which include this `taglist`")
	mountFlags.StringArrayVar(&mountOptions.Paths, "path", nil, "only consider snapshots which include this (absolute) `path`")

	snapshotTemplate := time.RFC3339

	// on windows some characters are not allowed in filenames therefor we
	// remove them from the template for snapshot names
	if runtime.GOOS == "windows" {
		reservedCharacters := regexp.MustCompile("[<>:\"/\\|?*]")
		snapshotTemplate = reservedCharacters.ReplaceAllString(snapshotTemplate, "")
	}

	mountFlags.StringVar(&mountOptions.SnapshotTemplate, "snapshot-template", snapshotTemplate, "set `template` to use for snapshot dirs")
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
			Warnf("unable to umount (maybe already umounted or still in use?): %v\n", err)
		}
		return nil
	})

	return mount(opts, gopts, mountpoint)
}
