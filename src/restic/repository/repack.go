package repository

import (
	"bytes"
	"io"
	"restic/backend"
	"restic/crypto"
	"restic/debug"
	"restic/pack"
)

// Repack takes a list of packs together with a list of blobs contained in
// these packs. Each pack is loaded and the blobs listed in keepBlobs is saved
// into a new pack. Afterwards, the packs are removed. This operation requires
// an exclusive lock on the repo.
func Repack(repo *Repository, packs backend.IDSet, keepBlobs pack.BlobSet) (err error) {
	debug.Log("Repack", "repacking %d packs while keeping %d blobs", len(packs), len(keepBlobs))

	buf := make([]byte, 0, maxPackSize)
	for packID := range packs {
		// load the complete pack
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
			h := pack.Handle{ID: entry.ID, Type: entry.Type}
			if !keepBlobs.Has(h) {
				continue
			}

			ciphertext := buf[entry.Offset : entry.Offset+entry.Length]

			if cap(plaintext) < len(ciphertext) {
				plaintext = make([]byte, len(ciphertext))
			}

			plaintext, err = crypto.Decrypt(repo.Key(), plaintext, ciphertext)
			if err != nil {
				return err
			}

			_, err = repo.SaveAndEncrypt(entry.Type, plaintext, &entry.ID)
			if err != nil {
				return err
			}

			debug.Log("Repack", "  saved blob %v", entry.ID.Str())

			keepBlobs.Delete(h)
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
