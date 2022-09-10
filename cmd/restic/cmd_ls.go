package main

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/fs"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/walker"
)

var cmdLs = &cobra.Command{
	Use:   "ls [flags] snapshotID [dir...]",
	Short: "List files in a snapshot",
	Long: `
The "ls" command lists files and directories in a snapshot.

The special snapshot ID "latest" can be used to list files and
directories of the latest snapshot in the repository. The
--host flag can be used in conjunction to select the latest
snapshot originating from a certain host only.

File listings can optionally be filtered by directories. Any
positional arguments after the snapshot ID are interpreted as
absolute directory paths, and only files inside those directories
will be listed. If the --recursive flag is used, then the filter
will allow traversing into matching directories' subfolders.
Any directory paths specified must be absolute (starting with
a path separator); paths use the forward slash '/' as separator.

EXIT STATUS
===========

Exit status is 0 if the command was successful, and non-zero if there was any error.
`,
	DisableAutoGenTag: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runLs(lsOptions, globalOptions, args)
	},
}

// LsOptions collects all options for the ls command.
type LsOptions struct {
	ListLong bool
	snapshotFilterOptions
	Recursive bool
}

var lsOptions LsOptions

func init() {
	cmdRoot.AddCommand(cmdLs)

	flags := cmdLs.Flags()
	initSingleSnapshotFilterOptions(flags, &lsOptions.snapshotFilterOptions)
	flags.BoolVarP(&lsOptions.ListLong, "long", "l", false, "use a long listing format showing size and mode")
	flags.BoolVar(&lsOptions.Recursive, "recursive", false, "include files in subfolders of the listed directories")
}

type lsSnapshot struct {
	*restic.Snapshot
	ID         *restic.ID `json:"id"`
	ShortID    string     `json:"short_id"`
	StructType string     `json:"struct_type"` // "snapshot"
}

// Print node in our custom JSON format, followed by a newline.
func lsNodeJSON(enc *json.Encoder, path string, node *restic.Node) error {
	n := &struct {
		Name        string      `json:"name"`
		Type        string      `json:"type"`
		Path        string      `json:"path"`
		UID         uint32      `json:"uid"`
		GID         uint32      `json:"gid"`
		Size        *uint64     `json:"size,omitempty"`
		Mode        os.FileMode `json:"mode,omitempty"`
		Permissions string      `json:"permissions,omitempty"`
		ModTime     time.Time   `json:"mtime,omitempty"`
		AccessTime  time.Time   `json:"atime,omitempty"`
		ChangeTime  time.Time   `json:"ctime,omitempty"`
		StructType  string      `json:"struct_type"` // "node"

		size uint64 // Target for Size pointer.
	}{
		Name:        node.Name,
		Type:        node.Type,
		Path:        path,
		UID:         node.UID,
		GID:         node.GID,
		size:        node.Size,
		Mode:        node.Mode,
		Permissions: node.Mode.String(),
		ModTime:     node.ModTime,
		AccessTime:  node.AccessTime,
		ChangeTime:  node.ChangeTime,
		StructType:  "node",
	}
	// Always print size for regular files, even when empty,
	// but never for other types.
	if node.Type == "file" {
		n.Size = &n.size
	}

	return enc.Encode(n)
}

func runLs(opts LsOptions, gopts GlobalOptions, args []string) error {
	if len(args) == 0 {
		return errors.Fatal("no snapshot ID specified, specify snapshot ID or use special ID 'latest'")
	}

	// extract any specific directories to walk
	var dirs []string
	if len(args) > 1 {
		dirs = args[1:]
		for _, dir := range dirs {
			if !strings.HasPrefix(dir, "/") {
				return errors.Fatal("All path filters must be absolute, starting with a forward slash '/'")
			}
		}
	}

	withinDir := func(nodepath string) bool {
		if len(dirs) == 0 {
			return true
		}

		for _, dir := range dirs {
			// we're within one of the selected dirs, example:
			//   nodepath: "/test/foo"
			//   dir:      "/test"
			if fs.HasPathPrefix(dir, nodepath) {
				return true
			}
		}
		return false
	}

	approachingMatchingTree := func(nodepath string) bool {
		if len(dirs) == 0 {
			return true
		}

		for _, dir := range dirs {
			// the current node path is a prefix for one of the
			// directories, so we're interested in something deeper in the
			// tree. Example:
			//   nodepath: "/test"
			//   dir:      "/test/foo"
			if fs.HasPathPrefix(nodepath, dir) {
				return true
			}
		}
		return false
	}

	repo, err := OpenRepository(gopts)
	if err != nil {
		return err
	}

	snapshotLister, err := backend.MemorizeList(gopts.ctx, repo.Backend(), restic.SnapshotFile)
	if err != nil {
		return err
	}

	if err = repo.LoadIndex(gopts.ctx); err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(gopts.ctx)
	defer cancel()

	var (
		printSnapshot func(sn *restic.Snapshot)
		printNode     func(path string, node *restic.Node)
	)

	if gopts.JSON {
		enc := json.NewEncoder(gopts.stdout)

		printSnapshot = func(sn *restic.Snapshot) {
			err := enc.Encode(lsSnapshot{
				Snapshot:   sn,
				ID:         sn.ID(),
				ShortID:    sn.ID().Str(),
				StructType: "snapshot",
			})
			if err != nil {
				Warnf("JSON encode failed: %v\n", err)
			}
		}

		printNode = func(path string, node *restic.Node) {
			err := lsNodeJSON(enc, path, node)
			if err != nil {
				Warnf("JSON encode failed: %v\n", err)
			}
		}
	} else {
		printSnapshot = func(sn *restic.Snapshot) {
			Verbosef("snapshot %s of %v filtered by %v at %s):\n", sn.ID().Str(), sn.Paths, dirs, sn.Time)
		}
		printNode = func(path string, node *restic.Node) {
			Printf("%s\n", formatNode(path, node, lsOptions.ListLong))
		}
	}

	for sn := range FindFilteredSnapshots(ctx, snapshotLister, repo, opts.Hosts, opts.Tags, opts.Paths, args[:1]) {
		printSnapshot(sn)

		err := walker.Walk(ctx, repo, *sn.Tree, nil, func(_ restic.ID, nodepath string, node *restic.Node, err error) (bool, error) {
			if err != nil {
				return false, err
			}
			if node == nil {
				return false, nil
			}

			if withinDir(nodepath) {
				// if we're within a dir, print the node
				printNode(nodepath, node)

				// if recursive listing is requested, signal the walker that it
				// should continue walking recursively
				if opts.Recursive {
					return false, nil
				}
			}

			// if there's an upcoming match deeper in the tree (but we're not
			// there yet), signal the walker to descend into any subdirs
			if approachingMatchingTree(nodepath) {
				return false, nil
			}

			// otherwise, signal the walker to not walk recursively into any
			// subdirs
			if node.Type == "dir" {
				return false, walker.ErrSkipNode
			}
			return false, nil
		})

		if err != nil {
			return err
		}
	}

	return nil
}
