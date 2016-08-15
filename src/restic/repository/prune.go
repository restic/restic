package repository

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"restic/backend"
	"restic/crypto"
	"restic/debug"
	"restic/pack"
	"restic/worker"
)

// Repack takes a list of packs together with a list of blobs contained in
// these packs. Each pack is loaded and the blobs listed in keepBlobs is saved
// into a new pack. Afterwards, the packs are removed. This operation requires
// an exclusive lock on the repo.
func Repack(repo *Repository, packs, keepBlobs backend.IDSet) (err error) {
	debug.Log("Repack", "repacking %d packs while keeping %d blobs", len(packs), len(keepBlobs))

	buf := make([]byte, 0, maxPackSize)
	for packID := range packs {
		// load the complete blob
		h := backend.Handle{Type: backend.Data, Name: packID.String()}

		l, err := repo.Backend().Load(h, buf[:cap(buf)], 0)
		if err == io.ErrUnexpectedEOF {
			err = nil
			buf = buf[:l]
		}

		if err != nil {
			return err
		}

		debug.Log("Repack", "pack %v loaded (%d bytes)", packID.Str(), len(buf))

		unpck, err := pack.NewUnpacker(repo.Key(), bytes.NewReader(buf))
		if err != nil {
			return err
		}

		debug.Log("Repack", "processing pack %v, blobs: %v", packID.Str(), len(unpck.Entries))
		var plaintext []byte
		for _, entry := range unpck.Entries {
			if !keepBlobs.Has(entry.ID) {
				continue
			}

			ciphertext := buf[entry.Offset : entry.Offset+entry.Length]
			plaintext, err = crypto.Decrypt(repo.Key(), plaintext, ciphertext)
			if err != nil {
				return err
			}

			_, err = repo.SaveAndEncrypt(entry.Type, plaintext, &entry.ID)
			if err != nil {
				return err
			}

			debug.Log("Repack", "  saved blob %v", entry.ID.Str())

			keepBlobs.Delete(entry.ID)
		}
	}

	if err := repo.Flush(); err != nil {
		return err
	}

	for packID := range packs {
		err := repo.Backend().Remove(backend.Data, packID.String())
		if err != nil {
			debug.Log("Repack", "error removing pack %v: %v", packID.Str(), err)
			return err
		}
		debug.Log("Repack", "removed pack %v", packID.Str())
	}

	return nil
}

const rebuildIndexWorkers = 10

type loadBlobsResult struct {
	packID  backend.ID
	entries []pack.Blob
}

// loadBlobsFromAllPacks sends the contents of all packs to ch.
func loadBlobsFromAllPacks(repo *Repository, ch chan<- worker.Job, done <-chan struct{}) {
	f := func(job worker.Job, done <-chan struct{}) (interface{}, error) {
		packID := job.Data.(backend.ID)
		entries, err := repo.ListPack(packID)
		return loadBlobsResult{
			packID:  packID,
			entries: entries,
		}, err
	}

	jobCh := make(chan worker.Job)
	wp := worker.New(rebuildIndexWorkers, f, jobCh, ch)

	go func() {
		for id := range repo.List(backend.Data, done) {
			jobCh <- worker.Job{Data: id}
		}
		close(jobCh)
	}()

	wp.Wait()
}

// RebuildIndex lists all packs in the repo, writes a new index and removes all
// old indexes. This operation should only be done with an exclusive lock in
// place.
func RebuildIndex(repo *Repository) error {
	debug.Log("RebuildIndex", "start rebuilding index")

	done := make(chan struct{})
	defer close(done)

	ch := make(chan worker.Job)
	go loadBlobsFromAllPacks(repo, ch, done)

	idx := NewIndex()
	for job := range ch {
		id := job.Data.(backend.ID)

		if job.Error != nil {
			fmt.Fprintf(os.Stderr, "error for pack %v: %v\n", id, job.Error)
			continue
		}

		res := job.Result.(loadBlobsResult)

		for _, entry := range res.entries {
			pb := PackedBlob{
				ID:     entry.ID,
				Type:   entry.Type,
				Length: entry.Length,
				Offset: entry.Offset,
				PackID: res.packID,
			}
			idx.Store(pb)
		}
	}

	oldIndexes := backend.NewIDSet()
	for id := range repo.List(backend.Index, done) {
		idx.AddToSupersedes(id)
		oldIndexes.Insert(id)
	}

	id, err := SaveIndex(repo, idx)
	if err != nil {
		debug.Log("RebuildIndex.RebuildIndex", "error saving index: %v", err)
		return err
	}
	debug.Log("RebuildIndex.RebuildIndex", "new index saved as %v", id.Str())

	for indexID := range oldIndexes {
		err := repo.Backend().Remove(backend.Index, indexID.String())
		if err != nil {
			fmt.Fprintf(os.Stderr, "unable to remove index %v: %v\n", indexID.Str(), err)
		}
	}

	return nil
}
