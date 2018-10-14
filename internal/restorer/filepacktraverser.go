package restorer

import (
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/restic"
)

type filePackTraverser struct {
	lookup func(restic.ID, restic.BlobType) ([]restic.PackedBlob, bool)
}

// iterates over all remaining packs of the file
func (t *filePackTraverser) forEachFilePack(file *fileInfo, fn func(packIdx int, packID restic.ID, packBlobs []restic.Blob) bool) error {
	if len(file.blobs) == 0 {
		return nil
	}

	getBlobPack := func(blobID restic.ID) (restic.PackedBlob, error) {
		packs, found := t.lookup(blobID, restic.DataBlob)
		if !found {
			return restic.PackedBlob{}, errors.Errorf("Unknown blob %s", blobID.String())
		}
		// TODO which pack to use if multiple packs have the blob?
		// MUST return the same pack for the same blob during the same execution
		return packs[0], nil
	}

	var prevPackID restic.ID
	var prevPackBlobs []restic.Blob
	packIdx := 0
	for _, blobID := range file.blobs {
		packedBlob, err := getBlobPack(blobID)
		if err != nil {
			return err
		}
		if !prevPackID.IsNull() && prevPackID != packedBlob.PackID {
			if !fn(packIdx, prevPackID, prevPackBlobs) {
				return nil
			}
			packIdx++
		}
		if prevPackID != packedBlob.PackID {
			prevPackID = packedBlob.PackID
			prevPackBlobs = make([]restic.Blob, 0)
		}
		prevPackBlobs = append(prevPackBlobs, packedBlob.Blob)
	}
	if len(prevPackBlobs) > 0 {
		fn(packIdx, prevPackID, prevPackBlobs)
	}
	return nil
}
