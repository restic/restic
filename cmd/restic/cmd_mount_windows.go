package main

import (
	"context"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/winfsp/go-winfsp"
	"github.com/winfsp/go-winfsp/gofs"

	"github.com/restic/restic/internal/data"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/fuse"
	"github.com/restic/restic/internal/global"
	"github.com/restic/restic/internal/ui"
	"github.com/restic/restic/internal/ui/progress"
)

// defaultTimeTemplate avoids colons, which are not allowed in Windows file names.
const defaultTimeTemplate = "2006-01-02T15-04-05Z0700"

func registerMountCommand(cmdRoot *cobra.Command, globalOptions *global.Options) {
	cmdRoot.AddCommand(newMountCommand(globalOptions))
}

func newMountCommand(globalOptions *global.Options) *cobra.Command {
	var opts MountOptions

	cmd := &cobra.Command{
		Use:   "mount [flags] mountpoint",
		Short: "Mount the repository",
		Long: `
The "mount" command mounts the repository read-only via WinFSP at the given
mountpoint. The mountpoint is either an unused drive letter such as "X:" or
a directory that does not exist yet. WinFSP (https://winfsp.dev) needs to be
installed.

Snapshot Directories
====================

If you need a different template for directories that contain snapshots,
you can pass a time template via --time-template and path templates via
--path-template.

Example time template:

    --time-template "2006-01-02_15-04-05"

You need to specify a sample format for exactly the following timestamp:

    Mon Jan 2 15:04:05 -0700 MST 2006

As colons are not allowed in Windows file names, the time template must not
contain any. For details please see the documentation for time.Format() at:
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
			finalizeSnapshotFilter(&opts.SnapshotFilter)
			return runMount(cmd.Context(), opts, *globalOptions, args, globalOptions.Term)
		},
	}

	opts.AddFlags(cmd.Flags())
	return cmd
}

// MountOptions collects all options for the mount command.
type MountOptions struct {
	data.SnapshotFilter
	TimeTemplate  string
	PathTemplates []string
}

func (opts *MountOptions) AddFlags(f *pflag.FlagSet) {
	initMultiSnapshotFilter(f, &opts.SnapshotFilter, true)

	f.StringArrayVar(&opts.PathTemplates, "path-template", nil, "set `template` for path names (can be specified multiple times)")
	f.StringVar(&opts.TimeTemplate, "snapshot-template", defaultTimeTemplate, "set `template` to use for snapshot dirs")
	f.StringVar(&opts.TimeTemplate, "time-template", defaultTimeTemplate, "set `template` to use for times")
	_ = f.MarkDeprecated("snapshot-template", "use --time-template")
}

func runMount(ctx context.Context, opts MountOptions, gopts global.Options, args []string, term ui.Terminal) error {
	printer := progress.NewTerminalPrinter(false, gopts.Verbosity, term)

	if opts.TimeTemplate == "" {
		return errors.Fatal("time template string cannot be empty")
	}

	if strings.HasPrefix(opts.TimeTemplate, "/") || strings.HasSuffix(opts.TimeTemplate, "/") {
		return errors.Fatal("time template string cannot start or end with '/'")
	}

	if strings.Contains(opts.TimeTemplate, ":") {
		return errors.Fatal("time template string cannot contain ':' on Windows")
	}

	if len(args) == 0 {
		return errors.Fatal("wrong number of parameters")
	}

	mountpoint := args[0]

	// fail early if WinFSP is missing, before asking for the password
	if err := winfsp.LoadWinFSP(); err != nil {
		return errors.Fatalf("unable to load WinFSP, is it installed (https://winfsp.dev)? %v", err)
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
		Filter:        opts.SnapshotFilter,
		TimeTemplate:  opts.TimeTemplate,
		PathTemplates: opts.PathTemplates,
	}
	root := fuse.NewWinFS(ctx, repo, cfg)
	// load repository before reporting the mountpoint
	printer.S("Loading snapshots...")
	_, err = root.Stat(`\`)
	if err != nil {
		return err
	}

	fs, err := winfsp.Mount(gofs.New(root), mountpoint,
		winfsp.FileSystemName("restic"),
		winfsp.Attributes(winfsp.FspFSAttributeReadOnlyVolume),
	)
	if err != nil {
		return errors.Fatalf("unable to mount at %s: %v", mountpoint, err)
	}

	printer.S("Now serving the repository at %s", mountpoint)
	printer.S("Use another terminal or tool to browse the contents of this folder.")
	printer.S("When finished, quit with Ctrl-c here.")
	debug.Log("serving mount at %v", mountpoint)

	<-ctx.Done()
	debug.Log("running umount cleanup handler for mount at %v", mountpoint)
	fs.Unmount()

	return ErrOK
}
