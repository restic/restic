package restorer

import (
	"bytes"
	"context"
	"io"
	"io/ioutil"
	"testing"

	"github.com/restic/restic/internal/crypto"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
)

type TestBlob struct {
	data string
	pack string
}

type TestFile struct {
	name  string
	blobs []TestBlob
}

type TestRepo struct {
	key *crypto.Key

	// pack names and ids
	packsNameToID map[string]restic.ID
	packsIDToName map[restic.ID]string
	packsIDToData map[restic.ID][]byte

	// blobs and files
	blobs              map[restic.ID][]restic.PackedBlob
	files              []*fileInfo
	filesPathToContent map[string]string

	//
	loader func(ctx context.Context, h restic.Handle, length int, offset int64, fn func(rd io.Reader) error) error

	//
	idx filePackTraverser
}

func (i *TestRepo) Lookup(blobID restic.ID, _ restic.BlobType) ([]restic.PackedBlob, bool) {
	packs, found := i.blobs[blobID]
	return packs, found
}

func (i *TestRepo) packName(pack *packInfo) string {
	return i.packsIDToName[pack.id]
}

func (i *TestRepo) packID(name string) restic.ID {
	return i.packsNameToID[name]
}

func (i *TestRepo) pack(queue *packQueue, name string) *packInfo {
	id := i.packsNameToID[name]
	return queue.packs[id]
}

func (i *TestRepo) fileContent(file *fileInfo) string {
	return i.filesPathToContent[file.location]
}

func newTestRepo(content []TestFile) *TestRepo {
	type Pack struct {
		name  string
		data  []byte
		blobs map[restic.ID]restic.Blob
	}
	packs := make(map[string]Pack)

	key := crypto.NewRandomKey()
	seal := func(data []byte) []byte {
		ciphertext := restic.NewBlobBuffer(len(data))
		ciphertext = ciphertext[:0] // truncate the slice
		nonce := crypto.NewRandomNonce()
		ciphertext = append(ciphertext, nonce...)
		return key.Seal(ciphertext, nonce, data, nil)
	}

	filesPathToContent := make(map[string]string)

	for _, file := range content {
		var content string
		for _, blob := range file.blobs {
			content += blob.data

			// get the pack, create as necessary
			var pack Pack
			var found bool
			if pack, found = packs[blob.pack]; !found {
				pack = Pack{name: blob.pack, blobs: make(map[restic.ID]restic.Blob)}
			}

			// calculate blob id and add to the pack as necessary
			blobID := restic.Hash([]byte(blob.data))
			if _, found := pack.blobs[blobID]; !found {
				blobData := seal([]byte(blob.data))
				pack.blobs[blobID] = restic.Blob{
					Type:   restic.DataBlob,
					ID:     blobID,
					Length: uint(len(blobData)),
					Offset: uint(len(pack.data)),
				}
				pack.data = append(pack.data, blobData...)
			}

			packs[blob.pack] = pack
		}
		filesPathToContent[file.name] = content
	}

	blobs := make(map[restic.ID][]restic.PackedBlob)
	packsIDToName := make(map[restic.ID]string)
	packsIDToData := make(map[restic.ID][]byte)
	packsNameToID := make(map[string]restic.ID)

	for _, pack := range packs {
		packID := restic.Hash(pack.data)
		packsIDToName[packID] = pack.name
		packsIDToData[packID] = pack.data
		packsNameToID[pack.name] = packID
		for blobID, blob := range pack.blobs {
			blobs[blobID] = append(blobs[blobID], restic.PackedBlob{Blob: blob, PackID: packID})
		}
	}

	var files []*fileInfo
	for _, file := range content {
		content := restic.IDs{}
		for _, blob := range file.blobs {
			content = append(content, restic.Hash([]byte(blob.data)))
		}
		files = append(files, &fileInfo{location: file.name, blobs: content})
	}

	repo := &TestRepo{
		key:                key,
		packsIDToName:      packsIDToName,
		packsIDToData:      packsIDToData,
		packsNameToID:      packsNameToID,
		blobs:              blobs,
		files:              files,
		filesPathToContent: filesPathToContent,
	}
	repo.idx = filePackTraverser{lookup: repo.Lookup}
	repo.loader = func(ctx context.Context, h restic.Handle, length int, offset int64, fn func(rd io.Reader) error) error {
		packID, err := restic.ParseID(h.Name)
		if err != nil {
			return err
		}
		rd := bytes.NewReader(repo.packsIDToData[packID][int(offset) : int(offset)+length])
		return fn(rd)
	}

	return repo
}

func restoreAndVerify(t *testing.T, tempdir string, content []TestFile) {
	repo := newTestRepo(content)

	r := newFileRestorer(tempdir, repo.loader, repo.key, repo.idx)
	r.files = repo.files

	r.restoreFiles(context.TODO(), func(path string, err error) {
		rtest.OK(t, errors.Wrapf(err, "unexpected error"))
	})

	for _, file := range repo.files {
		target := r.targetPath(file.location)
		data, err := ioutil.ReadFile(target)
		if err != nil {
			t.Errorf("unable to read file %v: %v", file.location, err)
			continue
		}

		rtest.Equals(t, false, r.filesWriter.writers.Contains(target))

		content := repo.fileContent(file)
		if !bytes.Equal(data, []byte(content)) {
			t.Errorf("file %v has wrong content: want %q, got %q", file.location, content, data)
		}
	}

	rtest.OK(t, nil)
}

func TestFileRestorerBasic(t *testing.T) {
	tempdir, cleanup := rtest.TempDir(t)
	defer cleanup()

	restoreAndVerify(t, tempdir, []TestFile{
		TestFile{
			name: "file1",
			blobs: []TestBlob{
				TestBlob{"data1-1", "pack1-1"},
				TestBlob{"data1-2", "pack1-2"},
			},
		},
		TestFile{
			name: "file2",
			blobs: []TestBlob{
				TestBlob{"data2-1", "pack2-1"},
				TestBlob{"data2-2", "pack2-2"},
			},
		},
	})
}
