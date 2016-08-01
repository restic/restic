package restic

import (
	"restic/backend"
	"restic/repository"
)

//  FindUsedBlobs traverse the tree ID and adds all seen blobs to blobs.
func findUsedBlobs(repo *repository.Repository, treeID backend.ID, blobs backend.IDSet) error {
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
			err := findUsedBlobs(repo, *node.Subtree, blobs)
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
	return blobs, findUsedBlobs(repo, treeID, blobs)
}
