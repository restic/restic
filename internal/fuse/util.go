// +build darwin freebsd linux windows

package fuse

import (
	"path/filepath"
	"time"

	"github.com/restic/restic/internal/restic"
	"golang.org/x/net/context"
)

const minSnapshotsReloadTime = 60 * time.Second

// search element in string list.
func isElem(e string, list []string) bool {
	for _, x := range list {
		if e == x {
			return true
		}
	}
	return false
}

func cleanupNodeName(name string) string {
	return filepath.Base(name)
}

// replaceSpecialNodes replaces nodes with name "." and "/" by their contents.
// Otherwise, the node is returned.
func replaceSpecialNodes(ctx context.Context, repo restic.Repository, node *restic.Node) ([]*restic.Node, error) {
	if node.Type != "dir" || node.Subtree == nil {
		return []*restic.Node{node}, nil
	}

	if node.Name != "." && node.Name != "/" {
		return []*restic.Node{node}, nil
	}

	tree, err := repo.LoadTree(ctx, *node.Subtree)
	if err != nil {
		return nil, err
	}

	return tree.Nodes, nil
}
