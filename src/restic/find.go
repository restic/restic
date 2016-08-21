package restic

import (
	"restic/backend"
	"restic/pack"
	"restic/repository"
)

// FindUsedBlobs traverses the tree ID and adds all seen blobs (trees and data
// blobs) to the set blobs. The tree blobs in the `seen` BlobSet will not be visited
// again.
func FindUsedBlobs(repo *repository.Repository, treeID backend.ID, blobs pack.BlobSet, seen pack.BlobSet) error {
	blobs.Insert(pack.Handle{ID: treeID, Type: pack.Tree})

	tree, err := LoadTree(repo, treeID)
	if err != nil {
		return err
	}

	for _, node := range tree.Nodes {
		switch node.Type {
		case "file":
			for _, blob := range node.Content {
				blobs.Insert(pack.Handle{ID: blob, Type: pack.Data})
			}
		case "dir":
			subtreeID := *node.Subtree
			h := pack.Handle{ID: subtreeID, Type: pack.Tree}
			if seen.Has(h) {
				continue
			}

			seen.Insert(h)

			err := FindUsedBlobs(repo, subtreeID, blobs, seen)
			if err != nil {
				return err
			}
		}
	}

	return nil
}
