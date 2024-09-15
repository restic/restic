package main

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/dump"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/restic"

	"github.com/spf13/cobra"
)

var cmdDump = &cobra.Command{
	Use:   "dump [flags] snapshotID file",
	Short: "Print a backed-up file to stdout",
	Long: `
The "dump" command extracts files from a snapshot from the repository. If a
single file is selected, it prints its contents to stdout. Folders are output
as a tar (default) or zip file containing the contents of the specified folder.
Pass "/" as file name to dump the whole snapshot as an archive file.

The special snapshotID "latest" can be used to use the latest snapshot in the
repository.

To include the folder content at the root of the archive, you can use the
"snapshotID:subfolder" syntax, where "subfolder" is a path within the
snapshot.

EXIT STATUS
===========

Exit status is 0 if the command was successful.
Exit status is 1 if there was any error.
Exit status is 10 if the repository does not exist.
Exit status is 11 if the repository is already locked.
Exit status is 12 if the password is incorrect.
`,
	GroupID:           cmdGroupDefault,
	DisableAutoGenTag: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runDump(cmd.Context(), dumpOptions, globalOptions, args)
	},
}

// DumpOptions collects all options for the dump command.
type DumpOptions struct {
	restic.SnapshotFilter
	Archive string
	Target  string
}

var dumpOptions DumpOptions

func init() {
	cmdRoot.AddCommand(cmdDump)

	flags := cmdDump.Flags()
	initSingleSnapshotFilter(flags, &dumpOptions.SnapshotFilter)
	flags.StringVarP(&dumpOptions.Archive, "archive", "a", "tar", "set archive `format` as \"tar\" or \"zip\"")
	flags.StringVarP(&dumpOptions.Target, "target", "t", "", "write the output to target `path`")
}

func splitPath(p string) []string {
	d, f := path.Split(p)
	if d == "" || d == "/" {
		return []string{f}
	}
	s := splitPath(path.Join("/", d))
	return append(s, f)
}

func cleanupPathList(pathComponentsList [][]string) [][]string {
	// Build a set of paths for quick lookup
	pathSet := make(map[string]struct{})

	for _, splittedPath := range pathComponentsList {
		pathStr := path.Join(splittedPath...)
		pathSet[pathStr] = struct{}{}
	}

	// Filter out subpaths that are covered by parent directories
	filteredList := [][]string{}

	for _, splittedPath := range pathComponentsList {
		isCovered := false
		// Check if any prefix of the path exists in the pathSet
		for i := len(splittedPath) - 1; i >= 1; i-- {
			prefixPath := path.Join(splittedPath[0:i]...)
			if _, exists := pathSet[prefixPath]; exists {
				isCovered = true
				break
			}
		}
		if !isCovered {
			filteredList = append(filteredList, splittedPath)
		}
	}

	return filteredList
}

func filterPathComponents(pathComponentsList [][]string, prefix string) [][]string {
	var filteredList [][]string
	for _, pathComponents := range pathComponentsList {
		if len(pathComponents) > 1 && pathComponents[0] == prefix {
			filteredList = append(filteredList, pathComponents[1:])
		}
	}
	return filteredList
}

func printFromTree(ctx context.Context, tree *restic.Tree, repo restic.BlobLoader, prefix string, pathComponentsList [][]string, d *dump.Dumper, canWriteArchiveFunc func() error) error {
	// If we print / we need to assume that there are multiple nodes at that
	// level in the tree.
	if pathComponentsList[0][0] == "" {
		if err := canWriteArchiveFunc(); err != nil {
			return err
		}
		return d.DumpTree(ctx, tree, "/")
	}

	// make a set of the nodes that we dumped
	dumpedNodes := make(map[string]struct{})

	for _, node := range tree.Nodes {
		if ctx.Err() != nil {
			return ctx.Err()
		}

	out:
		// iterate over all pathComponents
		for _, pathComponents := range pathComponentsList {
			item := filepath.Join(prefix, pathComponents[0])
			l := len(pathComponents)

			// If dumping something in the highest level it will just take the
			// first item it finds and dump that according to the switch case below.
			if node.Name == pathComponents[0] {
				switch {
				case l == 1 && node.Type == restic.NodeTypeFile:
					d.WriteNode(ctx, node)
					dumpedNodes[node.Name] = struct{}{}
					break out
				case l > 1 && node.Type == restic.NodeTypeDir:
					subtree, err := restic.LoadTree(ctx, repo, *node.Subtree)
					if err != nil {
						return errors.Wrapf(err, "cannot load subtree for %q", item)
					}

					newComponentsList := filterPathComponents(pathComponentsList, node.Name)

					err = printFromTree(ctx, subtree, repo, item, newComponentsList, d, canWriteArchiveFunc)
					if err != nil {
						return err
					}
					dumpedNodes[node.Name] = struct{}{}
					break out
				case node.Type == restic.NodeTypeDir:
					if err := canWriteArchiveFunc(); err != nil {
						return err
					}
					subtree, err := restic.LoadTree(ctx, repo, *node.Subtree)
					if err != nil {
						return err
					}
					d.DumpTree(ctx, subtree, item)
					dumpedNodes[node.Name] = struct{}{}
					break out
				case l > 1:
					return fmt.Errorf("%q should be a dir, but is a %q", item, node.Type)
				case node.Type != restic.NodeTypeFile:
					return fmt.Errorf("%q should be a file, but is a %q", item, node.Type)
				}
			}
		}
	}

	// Check if all paths were found in the snapshot
	for _, item := range pathComponentsList {
		if _, ok := dumpedNodes[item[0]]; !ok {
			return fmt.Errorf("path %q not found in snapshot", filepath.Join(prefix, item[0]))
		}
	}

	return nil
}

func runDump(ctx context.Context, opts DumpOptions, gopts GlobalOptions, args []string) error {
	if len(args) < 2 {
		return errors.Fatal("no file and no snapshot ID specified")
	}

	if opts.Archive == "" && opts.Compress {
		return errors.Fatal("compressing is only supported when dumping to an archive")
	}

	if len(args) > 2 && opts.Archive == "" {
		return errors.Fatal("multiple files can only be dumped to an archive")
	}

	switch opts.Archive {
	case "tar", "zip":
	default:
		return fmt.Errorf("unknown archive format %q", opts.Archive)
	}

	snapshotIDString := args[0]

	debug.Log("dump file started for %d paths from %q", (len(args) - 1), snapshotIDString)

	ctx, repo, unlock, err := openWithReadLock(ctx, gopts, gopts.NoLock)
	if err != nil {
		return err
	}
	defer unlock()

	sn, subfolder, err := (&restic.SnapshotFilter{
		Hosts: opts.Hosts,
		Paths: opts.Paths,
		Tags:  opts.Tags,
	}).FindLatest(ctx, repo, repo, snapshotIDString)
	if err != nil {
		return errors.Fatalf("failed to find snapshot: %v", err)
	}

	bar := newIndexProgress(gopts.Quiet, gopts.JSON)
	err = repo.LoadIndex(ctx, bar)
	if err != nil {
		return err
	}

	sn.Tree, err = restic.FindTreeDirectory(ctx, repo, sn.Tree, subfolder)
	if err != nil {
		return err
	}

	tree, err := restic.LoadTree(ctx, repo, *sn.Tree)
	if err != nil {
		return errors.Fatalf("loading tree for snapshot %q failed: %v", snapshotIDString, err)
	}

	outputFileWriter := os.Stdout
	canWriteArchiveFunc := checkStdoutArchive

	if opts.Target != "" {
		file, err := os.Create(opts.Target)
		if err != nil {
			return fmt.Errorf("cannot dump to file: %w", err)
		}
		defer func() {
			_ = file.Close()
		}()

		outputFileWriter = file
		canWriteArchiveFunc = func() error { return nil }
	}

	d := dump.New(opts.Archive, repo, outputFileWriter)
	defer d.Close()

	var splittedPathList [][]string

	for i := 1; i < len(args); i++ {
		pathToPrint := args[i]
		debug.Log("dump file %q from %q", pathToPrint, snapshotIDString)

		splittedPath := splitPath(path.Clean(pathToPrint))
		splittedPathList = append(splittedPathList, splittedPath)
	}

	splittedPathList = cleanupPathList(splittedPathList)

	err = printFromTree(ctx, tree, repo, "/", splittedPathList, d, canWriteArchiveFunc)
	if err != nil {
		return errors.Fatalf("cannot dump file: %v", err)
	}

	return nil
}

func checkStdoutArchive() error {
	if stdoutIsTerminal() {
		return fmt.Errorf("stdout is the terminal, please redirect output")
	}
	return nil
}
