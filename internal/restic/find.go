package restic

import "context"

// FindUsedBlobs traverses the tree ID and adds all seen blobs (trees and data
// blobs) to the set blobs. Already seen tree blobs will not be visited again.
func FindUsedBlobs(ctx context.Context, repo Repository, treeID ID, blobs BlobSet) error {
	blobs.Insert(BlobHandle{ID: treeID, Type: TreeBlob})

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
			subtreeID := *node.Subtree
			h := BlobHandle{ID: subtreeID, Type: TreeBlob}
			if blobs.Has(h) {
				continue
			}

			err := FindUsedBlobs(ctx, repo, subtreeID, blobs)
			if err != nil {
				return err
			}
		}
	}

	return nil
}
