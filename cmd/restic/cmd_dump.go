package main

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/restic"

	"github.com/spf13/cobra"
)

var cmdDump = &cobra.Command{
	Use:   "dump [flags] snapshotID file",
	Short: "Print a backed-up file to stdout",
	Long: `
The "dump" command extracts a single file from a snapshot from the repository and
prints its contents to stdout.

The special snapshot "latest" can be used to use the latest snapshot in the
repository.
`,
	DisableAutoGenTag: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runDump(dumpOptions, globalOptions, args)
	},
}

// DumpOptions collects all options for the dump command.
type DumpOptions struct {
	Host  string
	Paths []string
	Tags  restic.TagLists
}

var dumpOptions DumpOptions

func init() {
	cmdRoot.AddCommand(cmdDump)

	flags := cmdDump.Flags()
	flags.StringVarP(&dumpOptions.Host, "host", "H", "", `only consider snapshots for this host when the snapshot ID is "latest"`)
	flags.Var(&dumpOptions.Tags, "tag", "only consider snapshots which include this `taglist` for snapshot ID \"latest\"")
	flags.StringArrayVar(&dumpOptions.Paths, "path", nil, "only consider snapshots which include this (absolute) `path` for snapshot ID \"latest\"")
}

func splitPath(p string) []string {
	d, f := path.Split(p)
	if d == "" || d == "/" {
		return []string{f}
	}
	s := splitPath(path.Clean(d))
	return append(s, f)
}

func dumpNode(ctx context.Context, repo restic.Repository, node *restic.Node) error {
	var buf []byte
	for _, id := range node.Content {
		size, found := repo.LookupBlobSize(id, restic.DataBlob)
		if !found {
			return errors.Errorf("id %v not found in repository", id)
		}

		buf = buf[:cap(buf)]
		if len(buf) < restic.CiphertextLength(int(size)) {
			buf = restic.NewBlobBuffer(int(size))
		}

		n, err := repo.LoadBlob(ctx, restic.DataBlob, id, buf)
		if err != nil {
			return err
		}
		buf = buf[:n]

		_, err = os.Stdout.Write(buf)
		if err != nil {
			return errors.Wrap(err, "Write")
		}
	}
	return nil
}

func printFromTree(ctx context.Context, tree *restic.Tree, repo restic.Repository, prefix string, pathComponents []string) error {
	if tree == nil {
		return fmt.Errorf("called with a nil tree")
	}
	if repo == nil {
		return fmt.Errorf("called with a nil repository")
	}
	l := len(pathComponents)
	if l == 0 {
		return fmt.Errorf("empty path components")
	}
	item := filepath.Join(prefix, pathComponents[0])
	for _, node := range tree.Nodes {
		if node.Name == pathComponents[0] {
			switch {
			case l == 1 && node.Type == "file":
				return dumpNode(ctx, repo, node)
			case l > 1 && node.Type == "dir":
				subtree, err := repo.LoadTree(ctx, *node.Subtree)
				if err != nil {
					return errors.Wrapf(err, "cannot load subtree for %q", item)
				}
				return printFromTree(ctx, subtree, repo, item, pathComponents[1:])
			case l > 1:
				return fmt.Errorf("%q should be a dir, but s a %q", item, node.Type)
			case node.Type != "file":
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

	snapshotIDString := args[0]
	pathToPrint := args[1]

	debug.Log("dump file %q from %q", pathToPrint, snapshotIDString)

	splittedPath := splitPath(pathToPrint)

	repo, err := OpenRepository(gopts)
	if err != nil {
		return err
	}

	if !gopts.NoLock {
		lock, err := lockRepo(repo)
		defer unlockRepo(lock)
		if err != nil {
			return err
		}
	}

	err = repo.LoadIndex(ctx)
	if err != nil {
		return err
	}

	var id restic.ID

	if snapshotIDString == "latest" {
		id, err = restic.FindLatestSnapshot(ctx, repo, opts.Paths, opts.Tags, opts.Host)
		if err != nil {
			Exitf(1, "latest snapshot for criteria not found: %v Paths:%v Host:%v", err, opts.Paths, opts.Host)
		}
	} else {
		id, err = restic.FindSnapshot(repo, snapshotIDString)
		if err != nil {
			Exitf(1, "invalid id %q: %v", snapshotIDString, err)
		}
	}

	sn, err := restic.LoadSnapshot(gopts.ctx, repo, id)
	if err != nil {
		Exitf(2, "loading snapshot %q failed: %v", snapshotIDString, err)
	}

	tree, err := repo.LoadTree(ctx, *sn.Tree)
	if err != nil {
		Exitf(2, "loading tree for snapshot %q failed: %v", snapshotIDString, err)
	}

	err = printFromTree(ctx, tree, repo, "", splittedPath)
	if err != nil {
		Exitf(2, "cannot dump file: %v", err)
	}

	return nil
}
