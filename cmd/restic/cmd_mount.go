//go:build darwin || freebsd || linux

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"golang.org/x/sys/unix"

	"github.com/restic/restic/internal/backend/local"
	"github.com/restic/restic/internal/backend/location"
	"github.com/restic/restic/internal/data"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/global"
	"github.com/restic/restic/internal/ui"

	"github.com/restic/restic/internal/fuse"

	systemFuse "github.com/anacrolix/fuse"
	"github.com/anacrolix/fuse/fs"
)

func registerMountCommand(cmdRoot *cobra.Command, globalOptions *global.Options) {
	cmdRoot.AddCommand(newMountCommand(globalOptions))
}

func newMountCommand(globalOptions *global.Options) *cobra.Command {
	var opts MountOptions

	cmd := &cobra.Command{
		Use:   "mount [flags] mountpoint",
		Short: "Mount the repository",
		Long: `
The "mount" command mounts the repository read-only via FUSE at the given
mountpoint.

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
			finalizeSnapshotFilter(&opts.SnapshotFilter)
			return runMount(cmd.Context(), opts, *globalOptions, args, globalOptions.Term)
		},
	}

	opts.AddFlags(cmd.Flags())
	return cmd
}

// MountOptions collects all options for the mount command.
type MountOptions struct {
	OwnerRoot            bool
	AllowOther           bool
	NoDefaultPermissions bool
	data.SnapshotFilter
	TimeTemplate  string
	PathTemplates []string
}

func (opts *MountOptions) AddFlags(f *pflag.FlagSet) {
	f.BoolVar(&opts.OwnerRoot, "owner-root", false, "use 'root' as the owner of files and dirs")
	f.BoolVar(&opts.AllowOther, "allow-other", false, "allow other users to access the data in the mounted directory")
	f.BoolVar(&opts.NoDefaultPermissions, "no-default-permissions", false, "for 'allow-other', ignore Unix permissions and allow users to read all snapshot files")

	initMultiSnapshotFilter(f, &opts.SnapshotFilter, true)

	f.StringArrayVar(&opts.PathTemplates, "path-template", nil, "set `template` for path names (can be specified multiple times)")
	f.StringVar(&opts.TimeTemplate, "snapshot-template", time.RFC3339, "set `template` to use for snapshot dirs")
	f.StringVar(&opts.TimeTemplate, "time-template", time.RFC3339, "set `template` to use for times")
	_ = f.MarkDeprecated("snapshot-template", "use --time-template")
}

func runMount(ctx context.Context, opts MountOptions, gopts global.Options, args []string, term ui.Terminal) error {
	printer := ui.NewProgressPrinter(false, gopts.Verbosity, term)

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
	if err := validateMountpoint(mountpoint, gopts); err != nil {
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
	// load repository before reporting the mountpoint
	printer.S("Loading snapshots...")
	_, err = root.ReadDirAll(ctx)
	if err != nil {
		return err
	}

	done := make(chan struct{})

	go func() {
		defer close(done)
		err = fs.Serve(c, root)
	}()

	printer.S("Now serving the repository at %s", mountpoint)
	printer.S("Use another terminal or tool to browse the contents of this folder.")
	printer.S("When finished, quit with Ctrl-c here or umount the mountpoint.")
	debug.Log("serving mount at %v", mountpoint)

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

func validateMountpoint(mountpoint string, gopts global.Options) error {
	// Check the existence of the mount point at the earliest stage to
	// prevent unnecessary computations while opening the repository.
	stat, err := os.Stat(mountpoint)
	if errors.Is(err, os.ErrNotExist) {
		return errors.Fatal(fmt.Sprintf("mountpoint %s does not exist", mountpoint))
	} else if err != nil {
		return errors.Fatal(fmt.Sprintf("mountpoint %s is inaccessible: %v", mountpoint, err))
	} else if !stat.IsDir() {
		return errors.Fatal(fmt.Sprintf("mountpoint %s is not a directory", mountpoint))
	}

	err = unix.Access(mountpoint, unix.W_OK|unix.X_OK)
	if err != nil {
		return errors.Fatal(fmt.Sprintf("mountpoint %s is not writeable or not executable", mountpoint))
	}

	// Refuse to mount onto (or under, or over) the local repository directory.
	// Doing so makes the FUSE server read its own backend files through the
	// mount it just created, deadlocking the kernel (GH #5234).
	loc, err := location.Parse(gopts.Backends, gopts.Repo)
	if err != nil {
		return err
	}
	if loc.Scheme == "local" {
		if err := checkMountpointOverlap(loc.Config.(*local.Config).Path, mountpoint); err != nil {
			return err
		}
	}
	return nil
}

// checkMountpointOverlap returns an error.Fatal if the local repository at
// repoPath and the mountpoint overlap: equal paths, mountpoint nested inside
// the repo, or the repo nested inside the mountpoint. Any overlap deadlocks
// the FUSE server (GH #5234).
func checkMountpointOverlap(repoPath, mountpoint string) error {
	rp, err := resolvePath(repoPath)
	if err != nil {
		return err
	}
	mp, err := resolvePath(mountpoint)
	if err != nil {
		return err
	}

	const tail = "; refusing to mount to avoid deadlocking the FUSE server"
	switch {
	case rp == mp:
		return errors.Fatal(fmt.Sprintf("mountpoint %s is the local repository directory%s", mp, tail))
	case isInside(rp, mp):
		return errors.Fatal(fmt.Sprintf("mountpoint %s is inside the local repository directory %s%s", mp, rp, tail))
	case isInside(mp, rp):
		return errors.Fatal(fmt.Sprintf("local repository directory %s is inside the mountpoint %s%s", rp, mp, tail))
	}
	return nil
}

// resolvePath returns p as an absolute, symlink-resolved path. If EvalSymlinks
// fails (e.g. the path does not fully exist), it falls back to the absolute
// form: overlap detection is best-effort and we'd rather refuse a clear
// overlap than abort on an unrelated stat error.
func resolvePath(p string) (string, error) {
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return abs, nil
	}
	return resolved, nil
}

// isInside reports whether child is strictly nested inside parent. Both paths
// must already be cleaned and absolute. Equal paths return false; the caller
// handles equality separately so it can produce a distinct error message.
func isInside(parent, child string) bool {
	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	if rel == "." || rel == ".." {
		return false
	}
	return !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
