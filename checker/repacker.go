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
	unusedBlobs backend.IDSet
	repo        *repository.Repository
}

// NewRepacker returns a new repacker that (when Repack() in run) cleans up the
// repository and creates new packs and indexs so that all blobs in unusedBlobs
// aren't used any more.
func NewRepacker(repo *repository.Repository, unusedBlobs backend.IDSet) *Repacker {
	return &Repacker{
		repo:        repo,
		unusedBlobs: unusedBlobs,
	}
}

// Repack runs the process of finding still used blobs in packs with unused
// blobs, extracts them and creates new packs with just the still-in-use blobs.
func (r *Repacker) Repack() error {
	debug.Log("Repacker.Repack", "searching packs for %v", r.unusedBlobs)

	unneededPacks, err := FindPacksForBlobs(r.repo, r.unusedBlobs)
	if err != nil {
		return err
	}

	debug.Log("Repacker.Repack", "found packs: %v", unneededPacks)

	blobs, err := FindBlobsForPacks(r.repo, unneededPacks)
	if err != nil {
		return err
	}

	debug.Log("Repacker.Repack", "found blobs: %v", blobs)

	for id := range r.unusedBlobs {
		debug.Log("Repacker.Repack", "remove unused blob %v", id.Str())
		blobs.Delete(id)
	}

	debug.Log("Repacker.Repack", "need to repack blobs: %v", blobs)

	err = RepackBlobs(r.repo, r.repo, blobs)
	if err != nil {
		return err
	}

	debug.Log("Repacker.Repack", "remove unneeded packs: %v", unneededPacks)
	for packID := range unneededPacks {
		err = r.repo.Backend().Remove(backend.Data, packID.String())
		if err != nil {
			return err
		}
	}

	debug.Log("Repacker.Repack", "rebuild index, unneeded packs: %v", unneededPacks)
	idx, err := r.repo.Index().RebuildIndex(unneededPacks)

	newIndexID, err := repository.SaveIndex(r.repo, idx)
	debug.Log("Repacker.Repack", "saved new index at %v, err %v", newIndexID.Str(), err)
	if err != nil {
		return err
	}

	debug.Log("Repacker.Repack", "remove old indexes: %v", idx.Supersedes())
	for _, id := range idx.Supersedes() {
		err = r.repo.Backend().Remove(backend.Index, id.String())
		if err != nil {
			debug.Log("Repacker.Repack", "error removing index %v: %v", id.Str(), err)
			return err
		}

		debug.Log("Repacker.Repack", "removed index %v", id.Str())
	}

	return nil
}

// FindPacksForBlobs returns the set of packs that contain the blobs.
func FindPacksForBlobs(repo *repository.Repository, blobs backend.IDSet) (backend.IDSet, error) {
	packs := backend.NewIDSet()
	idx := repo.Index()
	for id := range blobs {
		blob, err := idx.Lookup(id)
		if err != nil {
			return nil, err
		}

		packs.Insert(blob.PackID)
	}

	return packs, nil
}

// FindBlobsForPacks returns the set of blobs contained in a pack of packs.
func FindBlobsForPacks(repo *repository.Repository, packs backend.IDSet) (backend.IDSet, error) {
	blobs := backend.NewIDSet()

	for packID := range packs {
		for _, packedBlob := range repo.Index().ListPack(packID) {
			blobs.Insert(packedBlob.ID)
		}
	}

	return blobs, nil
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
func RepackBlobs(src, dst *repository.Repository, blobIDs backend.IDSet) (err error) {
	for id := range blobIDs {
		err = repackBlob(src, dst, id)
		if err != nil {
			return err
		}
	}

	err = dst.Flush()
	if err != nil {
		return err
	}

	return nil
}
