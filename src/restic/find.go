package restic

// FindUsedBlobs traverses the tree ID and adds all seen blobs (trees and data
// blobs) to the set blobs. The tree blobs in the `seen` BlobSet will not be visited
// again.
func FindUsedBlobs(repo Repository, treeID ID, blobs BlobSet, seen BlobSet) error {
	blobs.Insert(BlobHandle{ID: treeID, Type: TreeBlob})

	tree, err := repo.LoadTree(treeID)
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
