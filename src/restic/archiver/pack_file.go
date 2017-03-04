package archiver

import (
	"os"
	"restic"
	"restic/debug"
	"restic/errors"
	"restic/fs"
	"restic/hashing"
	"restic/pack"
)

// PackFile handles a not yet finished pack file stored in a temporary file.
type PackFile struct {
	File   *os.File
	Packer *pack.Packer
	Hash   *hashing.Writer
}

// Save stores a pack file in the backend.
func (f PackFile) Save(be restic.Backend) (restic.ID, error) {
	debug.Log("save packer with %d blobs\n", f.Packer.Count())
	_, err := f.Packer.Finalize()
	if err != nil {
		return restic.ID{}, err
	}

	_, err = f.File.Seek(0, 0)
	if err != nil {
		return restic.ID{}, errors.Wrap(err, "Seek")
	}

	id := restic.IDFromHash(f.Hash.Sum(nil))
	h := restic.Handle{Type: restic.DataFile, Name: id.String()}

	err = be.Save(h, f.File)
	if err != nil {
		debug.Log("Save(%v) error: %v", h, err)
		return restic.ID{}, err
	}

	debug.Log("saved as %v", h)

	err = f.File.Close()
	if err != nil {
		return restic.ID{}, errors.Wrap(err, "close tempfile")
	}

	err = fs.Remove(f.File.Name())
	if err != nil {
		return restic.ID{}, errors.Wrap(err, "Remove")
	}

	return id, nil
}
