package restorer

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/restic/restic/internal/crypto"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/repository"
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
	loader repository.BackendLoadFn
}

func (i *TestRepo) Lookup(bh restic.BlobHandle) []restic.PackedBlob {
	packs := i.blobs[bh.ID]
	return packs
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
		ciphertext := crypto.NewBlobBuffer(len(data))
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
					BlobHandle: restic.BlobHandle{
						Type: restic.DataBlob,
						ID:   blobID,
					},
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

func restoreAndVerify(t *testing.T, tempdir string, content []TestFile, files map[string]bool, sparse bool) {
	repo := newTestRepo(content)

	r := newFileRestorer(tempdir, repo.loader, repo.key, repo.Lookup, 2, sparse)

	if files == nil {
		r.files = repo.files
	} else {
		for _, file := range repo.files {
			if files[file.location] {
				r.files = append(r.files, file)
			}
		}
	}

	err := r.restoreFiles(context.TODO())
	rtest.OK(t, err)

	verifyRestore(t, r, repo)
}

func verifyRestore(t *testing.T, r *fileRestorer, repo *TestRepo) {
	for _, file := range r.files {
		target := r.targetPath(file.location)
		data, err := os.ReadFile(target)
		if err != nil {
			t.Errorf("unable to read file %v: %v", file.location, err)
			continue
		}

		content := repo.fileContent(file)
		if !bytes.Equal(data, []byte(content)) {
			t.Errorf("file %v has wrong content: want %q, got %q", file.location, content, data)
		}
	}
}

func TestFileRestorerBasic(t *testing.T) {
	tempdir := rtest.TempDir(t)

	for _, sparse := range []bool{false, true} {
		restoreAndVerify(t, tempdir, []TestFile{
			{
				name: "file1",
				blobs: []TestBlob{
					{"data1-1", "pack1-1"},
					{"data1-2", "pack1-2"},
				},
			},
			{
				name: "file2",
				blobs: []TestBlob{
					{"data2-1", "pack2-1"},
					{"data2-2", "pack2-2"},
				},
			},
			{
				name: "file3",
				blobs: []TestBlob{
					// same blob multiple times
					{"data3-1", "pack3-1"},
					{"data3-1", "pack3-1"},
				},
			},
		}, nil, sparse)
	}
}

func TestFileRestorerPackSkip(t *testing.T) {
	tempdir := rtest.TempDir(t)

	files := make(map[string]bool)
	files["file2"] = true

	for _, sparse := range []bool{false, true} {
		restoreAndVerify(t, tempdir, []TestFile{
			{
				name: "file1",
				blobs: []TestBlob{
					{"data1-1", "pack1"},
					{"data1-2", "pack1"},
					{"data1-3", "pack1"},
					{"data1-4", "pack1"},
					{"data1-5", "pack1"},
					{"data1-6", "pack1"},
				},
			},
			{
				name: "file2",
				blobs: []TestBlob{
					// file is contained in pack1 but need pack parts to be skipped
					{"data1-2", "pack1"},
					{"data1-4", "pack1"},
					{"data1-6", "pack1"},
				},
			},
		}, files, sparse)
	}
}

func TestErrorRestoreFiles(t *testing.T) {
	tempdir := rtest.TempDir(t)
	content := []TestFile{
		{
			name: "file1",
			blobs: []TestBlob{
				{"data1-1", "pack1-1"},
			},
		}}

	repo := newTestRepo(content)

	loadError := errors.New("load error")
	// loader always returns an error
	repo.loader = func(ctx context.Context, h restic.Handle, length int, offset int64, fn func(rd io.Reader) error) error {
		return loadError
	}

	r := newFileRestorer(tempdir, repo.loader, repo.key, repo.Lookup, 2, false)
	r.files = repo.files

	err := r.restoreFiles(context.TODO())
	rtest.Assert(t, errors.Is(err, loadError), "got %v, expected contained error %v", err, loadError)
}

func TestDownloadError(t *testing.T) {
	for i := 0; i < 100; i += 10 {
		testPartialDownloadError(t, i)
	}
}

func testPartialDownloadError(t *testing.T, part int) {
	tempdir := rtest.TempDir(t)
	content := []TestFile{
		{
			name: "file1",
			blobs: []TestBlob{
				{"data1-1", "pack1"},
				{"data1-2", "pack1"},
				{"data1-3", "pack1"},
			},
		}}

	repo := newTestRepo(content)

	// loader always returns an error
	loader := repo.loader
	repo.loader = func(ctx context.Context, h restic.Handle, length int, offset int64, fn func(rd io.Reader) error) error {
		// only load partial data to execise fault handling in different places
		err := loader(ctx, h, length*part/100, offset, fn)
		if err == nil {
			return nil
		}
		fmt.Println("Retry after error", err)
		return loader(ctx, h, length, offset, fn)
	}

	r := newFileRestorer(tempdir, repo.loader, repo.key, repo.Lookup, 2, false)
	r.files = repo.files
	r.Error = func(s string, e error) error {
		// ignore errors as in the `restore` command
		fmt.Println("error during restore", s, e)
		return nil
	}

	err := r.restoreFiles(context.TODO())
	rtest.OK(t, err)
	verifyRestore(t, r, repo)
}
