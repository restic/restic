package repository

import (
	"crypto/sha256"
	"io"
	"restic"
	"restic/crypto"
	"restic/debug"
	"restic/fs"
	"restic/hashing"
	"restic/pack"

	"restic/errors"
)

// Repack takes a list of packs together with a list of blobs contained in
// these packs. Each pack is loaded and the blobs listed in keepBlobs is saved
// into a new pack. Afterwards, the packs are removed. This operation requires
// an exclusive lock on the repo.
func Repack(repo restic.Repository, packs restic.IDSet, keepBlobs restic.BlobSet, p *restic.Progress) (err error) {
	debug.Log("repacking %d packs while keeping %d blobs", len(packs), len(keepBlobs))

	for packID := range packs {
		// load the complete pack into a temp file
		h := restic.Handle{Type: restic.DataFile, Name: packID.String()}

		tempfile, err := fs.TempFile("", "restic-temp-repack-")
		if err != nil {
			return errors.Wrap(err, "TempFile")
		}

		beRd, err := repo.Backend().Load(h, 0, 0)
		if err != nil {
			return err
		}

		hrd := hashing.NewReader(beRd, sha256.New())
		packLength, err := io.Copy(tempfile, hrd)
		if err != nil {
			return errors.Wrap(err, "Copy")
		}

		if err = beRd.Close(); err != nil {
			return errors.Wrap(err, "Close")
		}

		hash := restic.IDFromHash(hrd.Sum(nil))
		debug.Log("pack %v loaded (%d bytes), hash %v", packID.Str(), packLength, hash.Str())

		if !packID.Equal(hash) {
			return errors.Errorf("hash does not match id: want %v, got %v", packID, hash)
		}

		_, err = tempfile.Seek(0, 0)
		if err != nil {
			return errors.Wrap(err, "Seek")
		}

		blobs, err := pack.List(repo.Key(), tempfile, packLength)
		if err != nil {
			return err
		}

		debug.Log("processing pack %v, blobs: %v", packID.Str(), len(blobs))
		var buf []byte
		for _, entry := range blobs {
			h := restic.BlobHandle{ID: entry.ID, Type: entry.Type}
			if !keepBlobs.Has(h) {
				continue
			}

			debug.Log("  process blob %v", h)

			buf = buf[:len(buf)]
			if uint(len(buf)) < entry.Length {
				buf = make([]byte, entry.Length)
			}
			buf = buf[:entry.Length]

			n, err := tempfile.ReadAt(buf, int64(entry.Offset))
			if err != nil {
				return errors.Wrap(err, "ReadAt")
			}

			if n != len(buf) {
				return errors.Errorf("read blob %v from %v: not enough bytes read, want %v, got %v",
					h, tempfile.Name(), len(buf), n)
			}

			n, err = crypto.Decrypt(repo.Key(), buf, buf)
			if err != nil {
				return err
			}

			buf = buf[:n]

			id := restic.Hash(buf)
			if !id.Equal(entry.ID) {
				return errors.Errorf("read blob %v from %v: wrong data returned, hash is %v",
					h, tempfile.Name(), id)
			}

			_, err = repo.SaveBlob(entry.Type, buf, entry.ID)
			if err != nil {
				return err
			}

			debug.Log("  saved blob %v", entry.ID.Str())

			keepBlobs.Delete(h)
		}

		if err = tempfile.Close(); err != nil {
			return errors.Wrap(err, "Close")
		}

		if err = fs.RemoveIfExists(tempfile.Name()); err != nil {
			return errors.Wrap(err, "Remove")
		}
		if p != nil {
			p.Report(restic.Stat{Blobs: 1})
		}
	}

	if err := repo.Flush(); err != nil {
		return err
	}

	for packID := range packs {
		h := restic.Handle{Type: restic.DataFile, Name: packID.String()}
		err := repo.Backend().Remove(h)
		if err != nil {
			debug.Log("error removing pack %v: %v", packID.Str(), err)
			return err
		}
		debug.Log("removed pack %v", packID.Str())
	}

	return nil
}
