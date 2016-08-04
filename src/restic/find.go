package restic

import (
	"restic/backend"
	"restic/pack"
	"restic/repository"
)

//  findUsedBlobs traverse the tree ID and adds all seen blobs to blobs.
func findUsedBlobs(repo *repository.Repository, treeID backend.ID, blobs pack.BlobSet, seen pack.BlobSet) error {
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

			err := findUsedBlobs(repo, subtreeID, blobs, seen)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// FindUsedBlobs traverses the tree ID and adds all seen blobs (trees and data blobs) to the set blobs.
func FindUsedBlobs(repo *repository.Repository, treeID backend.ID, blobs pack.BlobSet) error {
	return findUsedBlobs(repo, treeID, blobs, pack.NewBlobSet())
}
