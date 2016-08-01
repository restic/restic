package restic

import (
	"restic/backend"
	"restic/repository"
)

//  FindUsedBlobs traverse the tree ID and adds all seen blobs to blobs.
func findUsedBlobs(repo *repository.Repository, treeID backend.ID, blobs backend.IDSet, seen backend.IDSet) error {
	blobs.Insert(treeID)

	tree, err := LoadTree(repo, treeID)
	if err != nil {
		return err
	}

	for _, node := range tree.Nodes {
		switch node.Type {
		case "file":
			for _, blob := range node.Content {
				blobs.Insert(blob)
			}
		case "dir":
			subtreeID := *node.Subtree
			if seen.Has(subtreeID) {
				continue
			}

			seen.Insert(subtreeID)

			err := findUsedBlobs(repo, subtreeID, blobs, seen)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// FindUsedBlobs traverses the tree ID and returns a set of all blobs
// encountered.
func FindUsedBlobs(repo *repository.Repository, treeID backend.ID) (blobs backend.IDSet, err error) {
	blobs = backend.NewIDSet()
	return blobs, findUsedBlobs(repo, treeID, blobs, backend.NewIDSet())
}
