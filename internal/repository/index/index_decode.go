//go:build !go1.27

package index

import (
	"encoding/json"

	"github.com/restic/restic/internal/repository/pack"
	"github.com/restic/restic/internal/restic"
)

// decodeIndexJSON unserializes an index using encoding/json.
func decodeIndexJSON(buf []byte) (*Index, error) {
	idxJSON := &jsonIndex{}

	err := json.Unmarshal(buf, idxJSON)
	if err != nil {
		return nil, err
	}

	idx := NewIndex()
	for _, p := range idxJSON.Packs {
		packID := idx.addToPacks(p.ID)

		for _, blob := range p.Blobs {
			idx.store(packID, pack.Blob{
				BlobHandle: restic.BlobHandle{
					Type: blob.Type,
					ID:   blob.ID},
				Offset:             blob.Offset,
				Length:             blob.Length,
				UncompressedLength: blob.UncompressedLength,
			})
		}
	}
	return idx, nil
}
