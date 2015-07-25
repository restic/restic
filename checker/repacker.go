package checker

import (
	"errors"

	"github.com/restic/restic/backend"
	"github.com/restic/restic/debug"
	"github.com/restic/restic/repository"
)

// Repacker extracts still used blobs from packs with unused blobs and creates
// new packs.
type Repacker struct {
	unusedBlobs []backend.ID
	src, dst    *repository.Repository
}

// NewRepacker returns a new repacker that (when Repack() in run) cleans up the
// repository and creates new packs and indexs so that all blobs in unusedBlobs
// aren't used any more.
func NewRepacker(src, dst *repository.Repository, unusedBlobs []backend.ID) *Repacker {
	return &Repacker{
		src:         src,
		dst:         dst,
		unusedBlobs: unusedBlobs,
	}
}

// Repack runs the process of finding still used blobs in packs with unused
// blobs, extracts them and creates new packs with just the still-in-use blobs.
func (r *Repacker) Repack() error {
	debug.Log("Repacker.Repack", "searching packs for %v", r.unusedBlobs)
	packs, err := FindPacksforBlobs(r.src, r.unusedBlobs)
	if err != nil {
		return err
	}

	debug.Log("Repacker.Repack", "found packs: %v", packs)

	return nil
}

// FindPacksforBlobs returns the set of packs that contain the blobs.
func FindPacksforBlobs(repo *repository.Repository, blobs []backend.ID) (backend.IDSet, error) {
	packs := backend.NewIDSet()
	idx := repo.Index()
	for _, id := range blobs {
		blob, err := idx.Lookup(id)
		if err != nil {
			return nil, err
		}

		packs.Insert(blob.PackID)
	}

	return packs, nil
}

// repackBlob loads a single blob from src and saves it in dst.
func repackBlob(src, dst *repository.Repository, id backend.ID) error {
	blob, err := src.Index().Lookup(id)
	if err != nil {
		return err
	}

	debug.Log("RepackBlobs", "repacking blob %v, len %v", id.Str(), blob.PlaintextLength())

	buf := make([]byte, 0, blob.PlaintextLength())
	buf, err = src.LoadBlob(blob.Type, id, buf)
	if err != nil {
		return err
	}

	if uint(len(buf)) != blob.PlaintextLength() {
		debug.Log("RepackBlobs", "repack blob %v: len(buf) isn't equal to length: %v = %v", id.Str(), len(buf), blob.PlaintextLength())
		return errors.New("LoadBlob returned wrong data, len() doesn't match")
	}

	_, err = dst.SaveAndEncrypt(blob.Type, buf, &id)
	if err != nil {
		return err
	}

	return nil
}

// RepackBlobs reads all blobs in blobIDs from src and saves them into new pack
// files in dst. Source and destination repo may be the same.
func RepackBlobs(src, dst *repository.Repository, blobIDs backend.IDs) (err error) {
	for _, id := range blobIDs {
		err = repackBlob(src, dst, id)
		if err != nil {
			return err
		}
	}

	err = dst.Flush()
	if err != nil {
		return err
	}

	err = dst.SaveIndex()
	if err != nil {
		return err
	}

	return nil
}
