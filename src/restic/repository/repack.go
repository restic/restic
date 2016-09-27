package repository

import (
	"bytes"
	"io"
	"restic"
	"restic/crypto"
	"restic/debug"
	"restic/pack"

	"restic/errors"
)

// Repack takes a list of packs together with a list of blobs contained in
// these packs. Each pack is loaded and the blobs listed in keepBlobs is saved
// into a new pack. Afterwards, the packs are removed. This operation requires
// an exclusive lock on the repo.
func Repack(repo restic.Repository, packs restic.IDSet, keepBlobs restic.BlobSet) (err error) {
	debug.Log("repacking %d packs while keeping %d blobs", len(packs), len(keepBlobs))

	buf := make([]byte, 0, maxPackSize)
	for packID := range packs {
		// load the complete pack
		h := restic.Handle{Type: restic.DataFile, Name: packID.String()}

		l, err := repo.Backend().Load(h, buf[:cap(buf)], 0)
		if errors.Cause(err) == io.ErrUnexpectedEOF {
			err = nil
			buf = buf[:l]
		}

		if err != nil {
			return err
		}

		debug.Log("pack %v loaded (%d bytes)", packID.Str(), len(buf))

		blobs, err := pack.List(repo.Key(), bytes.NewReader(buf), int64(len(buf)))
		if err != nil {
			return err
		}

		debug.Log("processing pack %v, blobs: %v", packID.Str(), len(blobs))
		var plaintext []byte
		for _, entry := range blobs {
			h := restic.BlobHandle{ID: entry.ID, Type: entry.Type}
			if !keepBlobs.Has(h) {
				continue
			}

			debug.Log("  process blob %v", h)

			ciphertext := buf[entry.Offset : entry.Offset+entry.Length]
			plaintext = plaintext[:len(plaintext)]
			if len(plaintext) < len(ciphertext) {
				plaintext = make([]byte, len(ciphertext))
			}

			debug.Log("  ciphertext %d, plaintext %d", len(plaintext), len(ciphertext))

			n, err := crypto.Decrypt(repo.Key(), plaintext, ciphertext)
			if err != nil {
				return err
			}
			plaintext = plaintext[:n]

			_, err = repo.SaveBlob(entry.Type, plaintext, entry.ID)
			if err != nil {
				return err
			}

			debug.Log("  saved blob %v", entry.ID.Str())

			keepBlobs.Delete(h)
		}
	}

	if err := repo.Flush(); err != nil {
		return err
	}

	for packID := range packs {
		err := repo.Backend().Remove(restic.DataFile, packID.String())
		if err != nil {
			debug.Log("error removing pack %v: %v", packID.Str(), err)
			return err
		}
		debug.Log("removed pack %v", packID.Str())
	}

	return nil
}
