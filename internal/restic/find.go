package restic

import "context"

// TreeLoader loads a tree from a repository.
type TreeLoader interface {
	LoadTree(context.Context, ID) (*Tree, error)
}

// FindUsedBlobs traverses the tree ID and adds all seen blobs (trees and data
// blobs) to the set blobs. Already seen tree blobs will not be visited again.
func FindUsedBlobs(ctx context.Context, repo TreeLoader, treeID ID, blobs BlobSet) error {
	h := BlobHandle{ID: treeID, Type: TreeBlob}
	if blobs.Has(h) {
		return nil
	}
	blobs.Insert(h)

	tree, err := repo.LoadTree(ctx, treeID)
	if err != nil {
		return err
	}

	for _, node := range tree.Nodes {
		switch node.Type {
		case "file":
			for _, blob := range node.Content {
				blobs.Insert(BlobHandle{ID: blob, Type: DataBlob})
			}
		case "dir":
			err := FindUsedBlobs(ctx, repo, *node.Subtree, blobs)
			if err != nil {
				return err
			}
		}
	}

	return nil
}
