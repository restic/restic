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

///////////////////////////////////////////////////////////////////////////////
// test helpers (TODO move to a dedicated file?)
///////////////////////////////////////////////////////////////////////////////

type _Blob struct {
	data string
	pack string
}

type _File struct {
	name  string
	blobs []_Blob
}

type _TestData struct {
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

func (i *_TestData) Lookup(blobID restic.ID, _ restic.BlobType) ([]restic.PackedBlob, bool) {
	packs, found := i.blobs[blobID]
	return packs, found
}

func (i *_TestData) packName(pack *packInfo) string {
	return i.packsIDToName[pack.id]
}
func (i *_TestData) packID(name string) restic.ID {
	return i.packsNameToID[name]
}

func (i *_TestData) pack(queue *packQueue, name string) *packInfo {
	id := i.packsNameToID[name]
	return queue.packs[id]
}

func (i *_TestData) fileContent(file *fileInfo) string {
	return i.filesPathToContent[file.path]
}

func _newTestData(_files []_File) *_TestData {
	type _Pack struct {
		name  string
		data  []byte
		blobs map[restic.ID]restic.Blob
	}
	_packs := make(map[string]_Pack)

	key := crypto.NewRandomKey()
	seal := func(data []byte) []byte {
		ciphertext := restic.NewBlobBuffer(len(data))
		ciphertext = ciphertext[:0] // TODO what does this actually do?
		nonce := crypto.NewRandomNonce()
		ciphertext = append(ciphertext, nonce...)
		return key.Seal(ciphertext, nonce, data, nil)
	}

	filesPathToContent := make(map[string]string)

	for _, _file := range _files {
		var content string
		for _, _blob := range _file.blobs {
			content += _blob.data

			// get the pack, create as necessary
			var _pack _Pack
			var found bool // TODO is there more concise way of doing this in go?
			if _pack, found = _packs[_blob.pack]; !found {
				_pack = _Pack{name: _blob.pack, blobs: make(map[restic.ID]restic.Blob)}
			}

			// calculate blob id and add to the pack as necessary
			_blobID := restic.Hash([]byte(_blob.data))
			if _, found := _pack.blobs[_blobID]; !found {
				_blobData := seal([]byte(_blob.data))
				_pack.blobs[_blobID] = restic.Blob{
					Type:   restic.DataBlob,
					ID:     _blobID,
					Length: uint(len(_blobData)), // XXX is Length encrypted or plaintext?
					Offset: uint(len(_pack.data)),
				}
				_pack.data = append(_pack.data, _blobData...)
			}

			_packs[_blob.pack] = _pack
		}
		filesPathToContent[_file.name] = content
	}

	blobs := make(map[restic.ID][]restic.PackedBlob)
	packsIDToName := make(map[restic.ID]string)
	packsIDToData := make(map[restic.ID][]byte)
	packsNameToID := make(map[string]restic.ID)

	for _, _pack := range _packs {
		_packID := restic.Hash(_pack.data)
		packsIDToName[_packID] = _pack.name
		packsIDToData[_packID] = _pack.data
		packsNameToID[_pack.name] = _packID
		for blobID, blob := range _pack.blobs {
			blobs[blobID] = append(blobs[blobID], restic.PackedBlob{Blob: blob, PackID: _packID})
		}
	}

	var files []*fileInfo
	for _, _file := range _files {
		content := restic.IDs{}
		for _, _blob := range _file.blobs {
			content = append(content, restic.Hash([]byte(_blob.data)))
		}
		files = append(files, &fileInfo{path: _file.name, blobs: content})
	}

	_data := &_TestData{
		key:                key,
		packsIDToName:      packsIDToName,
		packsIDToData:      packsIDToData,
		packsNameToID:      packsNameToID,
		blobs:              blobs,
		files:              files,
		filesPathToContent: filesPathToContent,
	}
	_data.idx = filePackTraverser{lookup: _data.Lookup}
	_data.loader = func(ctx context.Context, h restic.Handle, length int, offset int64, fn func(rd io.Reader) error) error {
		packID, err := restic.ParseID(h.Name)
		if err != nil {
			return err
		}
		rd := bytes.NewReader(_data.packsIDToData[packID][int(offset) : int(offset)+length])
		return fn(rd)
	}

	return _data
}

func restoreAndVerify(t *testing.T, _files []_File) {
	test := _newTestData(_files)

	r := newFileRestorer(test.loader, test.key, test.idx)
	r.files = test.files

	r.restoreFiles(context.TODO(), func(path string, err error) {
		rtest.OK(t, errors.Wrapf(err, "unexpected error"))
	})

	for _, file := range test.files {
		data, err := ioutil.ReadFile(file.path)
		if err != nil {
			t.Errorf("unable to read file %v: %v", file.path, err)
			continue
		}

		rtest.Equals(t, false, r.filesWriter.writers.Contains(file.path))

		content := test.fileContent(file)
		if !bytes.Equal(data, []byte(content)) {
			t.Errorf("file %v has wrong content: want %q, got %q", file.path, content, data)
		}
	}

	rtest.OK(t, nil)
}

func TestFileRestorer_basic(t *testing.T) {
	tempdir, cleanup := rtest.TempDir(t)
	defer cleanup()

	restoreAndVerify(t, []_File{
		_File{
			name: tempdir + "/file1",
			blobs: []_Blob{
				_Blob{"data1-1", "pack1-1"},
				_Blob{"data1-2", "pack1-2"},
			},
		},
		_File{
			name: tempdir + "/file2",
			blobs: []_Blob{
				_Blob{"data2-1", "pack2-1"},
				_Blob{"data2-2", "pack2-2"},
			},
		},
	})
}

func TestFileRestorer_emptyFile(t *testing.T) {
	tempdir, cleanup := rtest.TempDir(t)
	defer cleanup()

	restoreAndVerify(t, []_File{
		_File{
			name:  tempdir + "/empty",
			blobs: []_Blob{},
		},
	})
}
