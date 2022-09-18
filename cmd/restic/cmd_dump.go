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

The special snapshot "latest" can be used to use the latest snapshot in the
repository.

EXIT STATUS
===========

Exit status is 0 if the command was successful, and non-zero if there was any error.
`,
	DisableAutoGenTag: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runDump(dumpOptions, globalOptions, args)
	},
}

// DumpOptions collects all options for the dump command.
type DumpOptions struct {
	snapshotFilterOptions
	Archive string
}

var dumpOptions DumpOptions

func init() {
	cmdRoot.AddCommand(cmdDump)

	flags := cmdDump.Flags()
	initMultiSnapshotFilterOptions(flags, &dumpOptions.snapshotFilterOptions, true)
	flags.StringVarP(&dumpOptions.Archive, "archive", "a", "tar", "set archive `format` as \"tar\" or \"zip\"")
}

func splitPath(p string) []string {
	d, f := path.Split(p)
	if d == "" || d == "/" {
		return []string{f}
	}
	s := splitPath(path.Join("/", d))
	return append(s, f)
}

func printFromTree(ctx context.Context, tree *restic.Tree, repo restic.Repository, prefix string, pathComponents []string, d *dump.Dumper) error {
	// If we print / we need to assume that there are multiple nodes at that
	// level in the tree.
	if pathComponents[0] == "" {
		if err := checkStdoutArchive(); err != nil {
			return err
		}
		return d.DumpTree(ctx, tree, "/")
	}

	item := filepath.Join(prefix, pathComponents[0])
	l := len(pathComponents)
	for _, node := range tree.Nodes {
		// If dumping something in the highest level it will just take the
		// first item it finds and dump that according to the switch case below.
		if node.Name == pathComponents[0] {
			switch {
			case l == 1 && dump.IsFile(node):
				return d.WriteNode(ctx, node)
			case l > 1 && dump.IsDir(node):
				subtree, err := restic.LoadTree(ctx, repo, *node.Subtree)
				if err != nil {
					return errors.Wrapf(err, "cannot load subtree for %q", item)
				}
				return printFromTree(ctx, subtree, repo, item, pathComponents[1:], d)
			case dump.IsDir(node):
				if err := checkStdoutArchive(); err != nil {
					return err
				}
				subtree, err := restic.LoadTree(ctx, repo, *node.Subtree)
				if err != nil {
					return err
				}
				return d.DumpTree(ctx, subtree, item)
			case l > 1:
				return fmt.Errorf("%q should be a dir, but is a %q", item, node.Type)
			case !dump.IsFile(node):
				return fmt.Errorf("%q should be a file, but is a %q", item, node.Type)
			}
		}
	}
	return fmt.Errorf("path %q not found in snapshot", item)
}

func runDump(opts DumpOptions, gopts GlobalOptions, args []string) error {
	ctx := gopts.ctx

	if len(args) != 2 {
		return errors.Fatal("no file and no snapshot ID specified")
	}

	switch opts.Archive {
	case "tar", "zip":
	default:
		return fmt.Errorf("unknown archive format %q", opts.Archive)
	}

	snapshotIDString := args[0]
	pathToPrint := args[1]

	debug.Log("dump file %q from %q", pathToPrint, snapshotIDString)

	splittedPath := splitPath(path.Clean(pathToPrint))

	repo, err := OpenRepository(gopts)
	if err != nil {
		return err
	}

	if !gopts.NoLock {
		lock, err := lockRepo(ctx, repo)
		defer unlockRepo(lock)
		if err != nil {
			return err
		}
	}

	var id restic.ID

	if snapshotIDString == "latest" {
		id, err = restic.FindLatestSnapshot(ctx, repo.Backend(), repo, opts.Paths, opts.Tags, opts.Hosts, nil)
		if err != nil {
			Exitf(1, "latest snapshot for criteria not found: %v Paths:%v Hosts:%v", err, opts.Paths, opts.Hosts)
		}
	} else {
		id, err = restic.FindSnapshot(ctx, repo.Backend(), snapshotIDString)
		if err != nil {
			Exitf(1, "invalid id %q: %v", snapshotIDString, err)
		}
	}

	sn, err := restic.LoadSnapshot(gopts.ctx, repo, id)
	if err != nil {
		Exitf(2, "loading snapshot %q failed: %v", snapshotIDString, err)
	}

	err = repo.LoadIndex(ctx)
	if err != nil {
		return err
	}

	tree, err := restic.LoadTree(ctx, repo, *sn.Tree)
	if err != nil {
		Exitf(2, "loading tree for snapshot %q failed: %v", snapshotIDString, err)
	}

	d := dump.New(opts.Archive, repo, os.Stdout)
	err = printFromTree(ctx, tree, repo, "/", splittedPath, d)
	if err != nil {
		Exitf(2, "cannot dump file: %v", err)
	}

	return nil
}

func checkStdoutArchive() error {
	if stdoutIsTerminal() {
		return fmt.Errorf("stdout is the terminal, please redirect output")
	}
	return nil
}
