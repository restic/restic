package main

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/fs"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/walker"
)

var cmdLs = &cobra.Command{
	Use:   "ls [flags] [snapshotID] [dir...]",
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
`,
	DisableAutoGenTag: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runLs(lsOptions, globalOptions, args)
	},
}

// LsOptions collects all options for the ls command.
type LsOptions struct {
	ListLong  bool
	Host      string
	Tags      restic.TagLists
	Paths     []string
	Recursive bool
}

var lsOptions LsOptions

func init() {
	cmdRoot.AddCommand(cmdLs)

	flags := cmdLs.Flags()
	flags.BoolVarP(&lsOptions.ListLong, "long", "l", false, "use a long listing format showing size and mode")
	flags.StringVarP(&lsOptions.Host, "host", "H", "", "only consider snapshots for this `host`, when no snapshot ID is given")
	flags.Var(&lsOptions.Tags, "tag", "only consider snapshots which include this `taglist`, when no snapshot ID is given")
	flags.StringArrayVar(&lsOptions.Paths, "path", nil, "only consider snapshots which include this (absolute) `path`, when no snapshot ID is given")
	flags.BoolVar(&lsOptions.Recursive, "recursive", false, "include files in subfolders of the listed directories")
}

type lsSnapshot struct {
	*restic.Snapshot
	ID         *restic.ID `json:"id"`
	ShortID    string     `json:"short_id"`
	StructType string     `json:"struct_type"` // "snapshot"
}

type lsNode struct {
	Name       string      `json:"name"`
	Type       string      `json:"type"`
	Path       string      `json:"path"`
	UID        uint32      `json:"uid"`
	GID        uint32      `json:"gid"`
	Size       uint64      `json:"size,omitempty"`
	Mode       os.FileMode `json:"mode,omitempty"`
	ModTime    time.Time   `json:"mtime,omitempty"`
	AccessTime time.Time   `json:"atime,omitempty"`
	ChangeTime time.Time   `json:"ctime,omitempty"`
	StructType string      `json:"struct_type"` // "node"
}

func runLs(opts LsOptions, gopts GlobalOptions, args []string) error {
	if len(args) == 0 && opts.Host == "" && len(opts.Tags) == 0 && len(opts.Paths) == 0 {
		return errors.Fatal("Invalid arguments, either give one or more snapshot IDs or set filters.")
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
			enc.Encode(lsSnapshot{
				Snapshot:   sn,
				ID:         sn.ID(),
				ShortID:    sn.ID().Str(),
				StructType: "snapshot",
			})
		}

		printNode = func(path string, node *restic.Node) {
			enc.Encode(lsNode{
				Name:       node.Name,
				Type:       node.Type,
				Path:       path,
				UID:        node.UID,
				GID:        node.GID,
				Size:       node.Size,
				Mode:       node.Mode,
				ModTime:    node.ModTime,
				AccessTime: node.AccessTime,
				ChangeTime: node.ChangeTime,
				StructType: "node",
			})
		}
	} else {
		printSnapshot = func(sn *restic.Snapshot) {
			Verbosef("snapshot %s of %v filtered by %v at %s):\n", sn.ID().Str(), sn.Paths, dirs, sn.Time)
		}
		printNode = func(path string, node *restic.Node) {
			Printf("%s\n", formatNode(path, node, lsOptions.ListLong))
		}
	}

	for sn := range FindFilteredSnapshots(ctx, repo, opts.Host, opts.Tags, opts.Paths, args[:1]) {
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
				return false, walker.SkipNode
			}
			return false, nil
		})

		if err != nil {
			return err
		}
	}

	return nil
}
