package main

import (
	"context"
	"path"
	"strings"

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

	repo, err := OpenRepository(gopts)
	if err != nil {
		return err
	}

	if err = repo.LoadIndex(gopts.ctx); err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(gopts.ctx)
	defer cancel()
	for sn := range FindFilteredSnapshots(ctx, repo, opts.Host, opts.Tags, opts.Paths, args[:1]) {
		Verbosef("snapshot %s of %v filtered by %v at %s):\n", sn.ID().Str(), sn.Paths, dirs, sn.Time)

		err := walker.Walk(ctx, repo, *sn.Tree, nil, func(nodepath string, node *restic.Node, err error) (bool, error) {
			if err != nil {
				return false, err
			}
			if node == nil {
				return false, nil
			}

			// apply any directory filters
			if len(dirs) > 0 {
				nodeDir := path.Dir(nodepath)

				// this first iteration ensures we do not traverse branches that
				// are not in matching trees or will not lead us to matching trees
				var walk bool
				for _, dir := range dirs {
					approachingMatchingTree := fs.HasPathPrefix(nodepath, dir)
					inMatchingTree := fs.HasPathPrefix(dir, nodepath)

					// this condition is complex, but it basically requires that we
					// are either approaching a matching tree (not yet deep enough)
					// or: if recursive, we have entered a matching tree; if non-
					// recursive, then that we are at exactly the right depth
					// (we can do the walk correctly by just using the condition of
					// "approachingMatchingTree || inMatchingTree", but it will be
					// much slower for non-recursive queries since it will continue
					// to traverse subtrees that are too deep and won't match -- this
					// extra check allows us to return SkipNode if we've gone TOO deep,
					// which skips all its subfolders)
					if approachingMatchingTree || opts.Recursive || (inMatchingTree && dir == nodeDir) {
						walk = true
						break
					}
				}
				if !walk {
					if node.Type == "dir" {
						// signal Walk() that it should not descend into the tree.
						return false, walker.SkipNode
					}

					// we must not return SkipNode for non-dir nodes because
					// then the remaining nodes in the same tree would be
					// skipped, so return nil instead
					return false, nil
				}

				// this second iteration ensures that we get an exact match
				// according to the filter and whether we should match subfolders
				var match bool
				for _, dir := range dirs {
					if nodepath == dir {
						// special case: match the directory filter exactly,
						// which may or may not be desirable depending on your
						// use case (for example, this is unnecessary when
						// wanting to simply list the contents of a folder,
						// rather than all files matching a directory prefix)
						match = true
						break
					}
					if opts.Recursive && fs.HasPathPrefix(dir, nodepath) {
						match = true
						break
					}
					if !opts.Recursive && nodeDir == dir {
						match = true
						break
					}
				}
				if !match {
					return false, nil
				}
			}

			Printf("%s\n", formatNode(nodepath, node, lsOptions.ListLong))

			return false, nil
		})
		if err != nil {
			return err
		}
	}
	return nil
}
