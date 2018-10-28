package repository

import (
	"context"
	"fmt"
	"os"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/fs"
	"github.com/restic/restic/internal/pack"
	"github.com/restic/restic/internal/restic"
)

// Repack takes a list of packs together with a list of blobs contained in
// these packs. Each pack is loaded and the blobs listed in keepBlobs is saved
// into a new pack. Returned is the list of obsolete packs which can then
// be removed.
func Repack(ctx context.Context, repo restic.Repository, packs restic.IDSet, keepBlobs restic.BlobSet, p *restic.Progress) (obsoletePacks restic.IDSet, err error) {
	debug.Log("repacking %d packs while keeping %d blobs", len(packs), len(keepBlobs))

	for packID := range packs {
		// load the complete pack into a temp file
		h := restic.Handle{Type: restic.DataFile, Name: packID.String()}

		tempfile, hash, packLength, err := DownloadAndHash(ctx, repo.Backend(), h)
		if err != nil {
			return nil, errors.Wrap(err, "Repack")
		}

		debug.Log("pack %v loaded (%d bytes), hash %v", packID, packLength, hash)

		if !packID.Equal(hash) {
			return nil, errors.Errorf("hash does not match id: want %v, got %v", packID, hash)
		}

		_, err = tempfile.Seek(0, 0)
		if err != nil {
			return nil, errors.Wrap(err, "Seek")
		}

		blobs, err := pack.List(repo.Key(), tempfile, packLength)
		if err != nil {
			return nil, err
		}

		debug.Log("processing pack %v, blobs: %v", packID, len(blobs))
		var buf []byte
		for _, entry := range blobs {
			h := restic.BlobHandle{ID: entry.ID, Type: entry.Type}
			if !keepBlobs.Has(h) {
				continue
			}

			debug.Log("  process blob %v", h)

			buf = buf[:]
			if uint(len(buf)) < entry.Length {
				buf = make([]byte, entry.Length)
			}
			buf = buf[:entry.Length]

			n, err := tempfile.ReadAt(buf, int64(entry.Offset))
			if err != nil {
				return nil, errors.Wrap(err, "ReadAt")
			}

			if n != len(buf) {
				return nil, errors.Errorf("read blob %v from %v: not enough bytes read, want %v, got %v",
					h, tempfile.Name(), len(buf), n)
			}

			nonce, ciphertext := buf[:repo.Key().NonceSize()], buf[repo.Key().NonceSize():]
			plaintext, err := repo.Key().Open(ciphertext[:0], nonce, ciphertext, nil)
			if err != nil {
				return nil, err
			}

			id := restic.Hash(plaintext)
			if !id.Equal(entry.ID) {
				debug.Log("read blob %v/%v from %v: wrong data returned, hash is %v",
					h.Type, h.ID, tempfile.Name(), id)
				fmt.Fprintf(os.Stderr, "read blob %v from %v: wrong data returned, hash is %v",
					h, tempfile.Name(), id)
			}

			_, err = repo.SaveBlob(ctx, entry.Type, plaintext, entry.ID)
			if err != nil {
				return nil, err
			}

			debug.Log("  saved blob %v", entry.ID)

			keepBlobs.Delete(h)
		}

		if err = tempfile.Close(); err != nil {
			return nil, errors.Wrap(err, "Close")
		}

		if err = fs.RemoveIfExists(tempfile.Name()); err != nil {
			return nil, errors.Wrap(err, "Remove")
		}
		if p != nil {
			p.Report(restic.Stat{Blobs: 1})
		}
	}

	if err := repo.Flush(ctx); err != nil {
		return nil, err
	}

	return packs, nil
}
