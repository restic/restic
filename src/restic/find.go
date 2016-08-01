package restic

import (
	"restic/backend"
	"restic/repository"
)

// FindUsedBlobs traverses the tree ID and returns a set of all blobs
// encountered.
func FindUsedBlobs(repo *repository.Repository, treeID backend.ID) (backend.IDSet, error) {
	return nil, nil
}
