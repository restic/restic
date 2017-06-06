package archiver

import (
	"crypto/sha256"
	"io/ioutil"
	"restic"
	"restic/crypto"
	"restic/debug"
	"restic/errors"
	"restic/hashing"
	"restic/pack"
)

const minPackSize = 4 * 1024 * 1024
const maxPackSize = 16 * 1024 * 1024
const maxPackers = 20

// ContentManager handles a list of open pack files, the recently uploaded pack
// files and new blobs that have been added to the repo.
type ContentManager struct {
	Key     *crypto.Key
	Backend restic.Backend

	Files []PackFile

	Packs []restic.Pack
	Blobs restic.BlobSet
}

// NewContentManager returns a new ContentManager.
func NewContentManager(be restic.Backend, key *crypto.Key) *ContentManager {
	return &ContentManager{
		Backend: be,
		Key:     key,
		Blobs:   restic.NewBlobSet(),
	}
}

// encryptAndAdd encrypts plaintext and then adds it to the packer.
func (cm *ContentManager) encryptAndAdd(p *pack.Packer, h restic.BlobHandle, plaintext []byte) error {
	buf := restic.NewBlobBuffer(restic.CiphertextLength(len(plaintext)))

	// encrypt blob
	ciphertext, err := crypto.Encrypt(cm.Key, buf, plaintext)
	if err != nil {
		return err
	}

	_, err = p.Add(h.Type, h.ID, ciphertext)
	return err
}

// AddNewBlob encrypts and adds the blob to a suitable packer. If it is already
// present in cm.Blobs, AddNewBlob returns immediately. After saving to a
// packer, the blob is added to cm.Blobs.
func (cm *ContentManager) AddNewBlob(h restic.BlobHandle, buf []byte) error {
	// return early if the blob was already added
	if cm.Blobs.Has(h) {
		return nil
	}

	debug.Log("add new blob %v", h)

	// try existing packers
	for _, f := range cm.Files {
		if f.Packer.Size()+uint(len(buf)) < maxPackSize {
			err := cm.encryptAndAdd(f.Packer, h, buf)
			if err != nil {
				return err
			}

			cm.Blobs.Insert(h)

			return nil
		}
	}

	// create a new packer
	tmpfile, err := ioutil.TempFile("", "restic-temp-pack-")
	if err != nil {
		return errors.Wrap(err, "ioutil.TempFile")
	}

	hw := hashing.NewWriter(tmpfile, sha256.New())
	p := pack.NewPacker(cm.Key, hw)

	cm.Files = append(cm.Files, PackFile{
		Packer: p,
		Hash:   hw,
		File:   tmpfile,
	})

	return cm.encryptAndAdd(p, h, buf)
}

// FullFile returns a pack file that can be uploaded to the repo. If no
// pack file needs to be uploaded, ok is set to false.
func (cm *ContentManager) FullFile() (file PackFile, ok bool) {
	// search for a nearly full pack file (80% of maxPackSize)
	for i, f := range cm.Files {
		if f.Packer.Size() >= maxPackSize/10*8 {
			cm.Files = append(cm.Files[:i], cm.Files[i+1:]...)
			return f, true
		}
	}

	// if less than maxPackers or no packers at all, nothing nedes to be done
	if len(cm.Files) == 0 || len(cm.Files) < maxPackers {
		return PackFile{}, false
	}

	// else return the largest pack file
	var (
		size uint
		pos  int
	)

	for i, f := range cm.Files {
		if f.Packer.Size() > size {
			size = f.Packer.Size()
			pos = i
		}
	}

	f := cm.Files[pos]
	cm.Files = append(cm.Files[:pos], cm.Files[pos+1:]...)
	return f, true
}

// SaveFullFile uploads at most one full pack file to the repo. If no pack file
// needs to be uploaded, nothing is done.
func (cm *ContentManager) SaveFullFile() error {
	f, ok := cm.FullFile()
	if !ok {
		return nil
	}

	return cm.saveFile(f)
}

// saveFile stores a pack file in the backend. The pack is added to cm.Packs.
func (cm *ContentManager) saveFile(f PackFile) error {
	packID, err := f.Save(cm.Backend)
	if err != nil {
		return err
	}

	cm.Packs = append(cm.Packs, restic.Pack{
		ID:    packID,
		Blobs: f.Packer.Blobs(),
	})

	return nil
}

// SaveAllFiles stores all remaining pack files in the repo. The first error is
// returned.
func (cm *ContentManager) SaveAllFiles() error {
	for _, f := range cm.Files {
		err := cm.saveFile(f)
		if err != nil {
			return err
		}
	}

	cm.Files = cm.Files[:0]
	return nil
}
