package main

import (
	"archive/tar"
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/walker"

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
	if d == "" {
		return []string{f}
	}
	if d == "/" {
		return []string{d}
	}
	s := splitPath(path.Clean(d))
	return append(s, f)
}

func printFromTree(ctx context.Context, tree *restic.Tree, repo restic.Repository, prefix string, pathComponents []string, pathToPrint string) error {

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
		if node.Name == pathComponents[0] || pathComponents[0] == "/" {
			switch {
			case l == 1 && node.Type == "file":
				return getNodeData(ctx, os.Stdout, repo, node)
			case l > 1 && node.Type == "dir":
				subtree, err := repo.LoadTree(ctx, *node.Subtree)
				if err != nil {
					return errors.Wrapf(err, "cannot load subtree for %q", item)
				}
				return printFromTree(ctx, subtree, repo, item, pathComponents[1:], pathToPrint)
			case node.Type == "dir":
				node.Path = pathToPrint
				return tarTree(ctx, repo, node, pathToPrint)
			case l > 1:
				return fmt.Errorf("%q should be a dir, but is a %q", item, node.Type)
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

	splittedPath := splitPath(path.Clean(pathToPrint))

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

	err = printFromTree(ctx, tree, repo, "", splittedPath, pathToPrint)
	if err != nil {
		Exitf(2, "cannot dump file: %v", err)
	}

	return nil
}

func getNodeData(ctx context.Context, output io.Writer, repo restic.Repository, node *restic.Node) error {
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

		_, err = output.Write(buf)
		if err != nil {
			return errors.Wrap(err, "Write")
		}

	}
	return nil
}

func tarTree(ctx context.Context, repo restic.Repository, rootNode *restic.Node, rootPath string) error {

	if stdoutIsTerminal() {
		return fmt.Errorf("stdout is the terminal, please redirect output")
	}

	tw := tar.NewWriter(os.Stdout)
	defer tw.Close()

	// If we want to dump "/" we'll need to add the name of the first node, too
	// as it would get lost otherwise.
	if rootNode.Path == "/" {
		rootNode.Path = path.Join(rootNode.Path, rootNode.Name)
		rootPath = rootNode.Path
	}

	// we know that rootNode is a folder and walker.Walk will already process
	// the next node, so we have to tar this one first, too
	if err := tarNode(ctx, tw, rootNode, repo); err != nil {
		return err
	}

	err := walker.Walk(ctx, repo, *rootNode.Subtree, nil, func(_ restic.ID, nodepath string, node *restic.Node, err error) (bool, error) {
		if err != nil {
			return false, err
		}
		if node == nil {
			return false, nil
		}

		node.Path = path.Join(rootPath, nodepath)

		if node.Type == "file" || node.Type == "symlink" || node.Type == "dir" {
			err := tarNode(ctx, tw, node, repo)
			if err != err {
				return false, err
			}
		}

		return false, nil
	})

	return err
}

func tarNode(ctx context.Context, tw *tar.Writer, node *restic.Node, repo restic.Repository) error {

	header := &tar.Header{
		Name:       node.Path,
		Size:       int64(node.Size),
		Mode:       int64(node.Mode),
		Uid:        int(node.UID),
		Gid:        int(node.GID),
		ModTime:    node.ModTime,
		AccessTime: node.AccessTime,
		ChangeTime: node.ChangeTime,
		PAXRecords: parseXattrs(node.ExtendedAttributes),
	}

	if node.Type == "symlink" {
		header.Typeflag = tar.TypeSymlink
		header.Linkname = node.LinkTarget
	}

	if node.Type == "dir" {
		header.Typeflag = tar.TypeDir
	}

	err := tw.WriteHeader(header)

	if err != nil {
		return errors.Wrap(err, "TarHeader ")
	}

	return getNodeData(ctx, tw, repo, node)

}

func parseXattrs(xattrs []restic.ExtendedAttribute) map[string]string {
	tmpMap := make(map[string]string)

	for _, attr := range xattrs {
		attrString := string(attr.Value)

		if strings.HasPrefix(attr.Name, "system.posix_acl_") {
			na := acl{}
			na.decode(attr.Value)

			if na.String() != "" {
				if strings.Contains(attr.Name, "system.posix_acl_access") {
					tmpMap["SCHILY.acl.access"] = na.String()
				} else if strings.Contains(attr.Name, "system.posix_acl_default") {
					tmpMap["SCHILY.acl.default"] = na.String()
				}
			}

		} else {
			tmpMap["SCHILY.xattr."+attr.Name] = attrString
		}
	}

	return tmpMap
}
