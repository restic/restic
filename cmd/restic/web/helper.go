package web

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/restorer"
)

func splitPath(path string) []string {
	d, f := filepath.Split(path)
	if d == "" || d == "/" {
		return []string{f}
	}
	s := splitPath(filepath.Clean(d))
	return append(s, f)
}

func dumpNode(ctx context.Context, repo restic.Repository, node *restic.Node, writer io.Writer) error {
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

		_, err = writer.Write(buf)
		if err != nil {
			return errors.Wrap(err, "Write")
		}
	}
	return nil
}

func findNode(ctx context.Context, tree *restic.Tree, repo restic.Repository, prefix string, pathComponents []string) (*restic.Node, error) {
	if tree == nil {
		return nil, fmt.Errorf("called with a nil tree")
	}
	if repo == nil {
		return nil, fmt.Errorf("called with a nil repository")
	}
	l := len(pathComponents)
	if l == 0 {
		return nil, fmt.Errorf("empty path components")
	}
	item := filepath.Join(prefix, pathComponents[0])
	for _, node := range tree.Nodes {
		if node.Name == pathComponents[0] {
			switch {
			case l == 1 && node.Type == "file":
				return node, nil // found
			case l > 1 && node.Type == "dir":
				subtree, err := repo.LoadTree(ctx, *node.Subtree)
				if err != nil {
					return nil, errors.Wrapf(err, "cannot load subtree for %q", item)
				}
				return findNode(ctx, subtree, repo, item, pathComponents[1:])
			case l > 1:
				return nil, fmt.Errorf("%q should be a dir, but s a %q", item, node.Type)
			case node.Type != "file":
				return nil, fmt.Errorf("%q should be a file, but is a %q", item, node.Type)
			}
		}
	}
	return nil, fmt.Errorf("path %q not found in snapshot", item)
}

func restore(repo restic.Repository, snapshotID restic.ID, files []string, target string) (int, error) {
	res, err := restorer.NewRestorer(repo, snapshotID)
	if err != nil {
		return 0, errors.Errorf("creating restorer failed: %v", err)
	}

	totalErrors := 0
	res.Error = func(dir string, node *restic.Node, err error) error {
		fmt.Fprintf(os.Stdout, "ignoring error for %s: %s\n", dir, err)
		totalErrors++
		return nil
	}

	compareFunc := func(files []string, item string) (bool, bool) {
		for _, f := range files {
			if strings.Compare(f, item) == 0 {
				return true, true
			} else if strings.HasPrefix(f, item) {
				return true, true
			} else if strings.HasPrefix(item, f) {
				return true, true
			}
		}
		return false, false

	}

	res.SelectFilter = func(item string, dstpath string, node *restic.Node) (selectedForRestore bool, childMayBeSelected bool) {
		selectedForRestore, childMayBeSelected = compareFunc(files, item)
		return selectedForRestore, childMayBeSelected && node.Type == "dir"
	}

	err = res.RestoreTo(context.TODO(), target)
	return totalErrors, err
}
