package restorer

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"sort"
	"testing"

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
	packsIDToData map[restic.ID][]byte

	// blobs and files
	blobs              map[restic.ID][]restic.PackedBlob
	files              []*fileInfo
	filesPathToContent map[string]string

	//
	loader blobsLoaderFn
}

func (i *TestRepo) Lookup(tpe restic.BlobType, id restic.ID) []restic.PackedBlob {
	packs := i.blobs[id]
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
				blobData := []byte(blob.data)
				pack.blobs[blobID] = restic.Blob{
					BlobHandle: restic.BlobHandle{
						Type: restic.DataBlob,
						ID:   blobID,
					},
					Length:             uint(len(blobData)),
					UncompressedLength: uint(len(blobData)),
					Offset:             uint(len(pack.data)),
				}
				pack.data = append(pack.data, blobData...)
			}

			packs[blob.pack] = pack
		}
		filesPathToContent[file.name] = content
	}

	blobs := make(map[restic.ID][]restic.PackedBlob)
	packsIDToData := make(map[restic.ID][]byte)

	for _, pack := range packs {
		packID := restic.Hash(pack.data)
		packsIDToData[packID] = pack.data
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
		packsIDToData:      packsIDToData,
		blobs:              blobs,
		files:              files,
		filesPathToContent: filesPathToContent,
	}
	repo.loader = func(ctx context.Context, packID restic.ID, blobs []restic.Blob, handleBlobFn func(blob restic.BlobHandle, buf []byte, err error) error) error {
		blobs = append([]restic.Blob{}, blobs...)
		sort.Slice(blobs, func(i, j int) bool {
			return blobs[i].Offset < blobs[j].Offset
		})

		for _, blob := range blobs {
			found := false
			for _, e := range repo.blobs[blob.ID] {
				if packID == e.PackID {
					found = true
					buf := repo.packsIDToData[packID][e.Offset : e.Offset+e.Length]
					err := handleBlobFn(e.BlobHandle, buf, nil)
					if err != nil {
						return err
					}
				}
			}
			if !found {
				return fmt.Errorf("missing blob: %v", blob)
			}
		}
		return nil
	}

	return repo
}

func restoreAndVerify(t *testing.T, tempdir string, content []TestFile, files map[string]bool, sparse bool) {
	t.Helper()
	repo := newTestRepo(content)

	r := newFileRestorer(tempdir, repo.loader, repo.Lookup, 2, sparse, false, nil)

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
	t.Helper()
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
			{
				name:  "empty",
				blobs: []TestBlob{},
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

func TestFileRestorerFrequentBlob(t *testing.T) {
	tempdir := rtest.TempDir(t)

	for _, sparse := range []bool{false, true} {
		blobs := []TestBlob{
			{"data1-1", "pack1-1"},
		}
		for i := 0; i < 10000; i++ {
			blobs = append(blobs, TestBlob{"a", "pack1-1"})
		}
		blobs = append(blobs, TestBlob{"end", "pack1-1"})

		restoreAndVerify(t, tempdir, []TestFile{
			{
				name:  "file1",
				blobs: blobs,
			},
		}, nil, sparse)
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
	repo.loader = func(ctx context.Context, packID restic.ID, blobs []restic.Blob, handleBlobFn func(blob restic.BlobHandle, buf []byte, err error) error) error {
		return loadError
	}

	r := newFileRestorer(tempdir, repo.loader, repo.Lookup, 2, false, false, nil)
	r.files = repo.files

	err := r.restoreFiles(context.TODO())
	rtest.Assert(t, errors.Is(err, loadError), "got %v, expected contained error %v", err, loadError)
}

func TestFatalDownloadError(t *testing.T) {
	tempdir := rtest.TempDir(t)
	content := []TestFile{
		{
			name: "file1",
			blobs: []TestBlob{
				{"data1-1", "pack1"},
				{"data1-2", "pack1"},
			},
		},
		{
			name: "file2",
			blobs: []TestBlob{
				{"data2-1", "pack1"},
				{"data2-2", "pack1"},
				{"data2-3", "pack1"},
			},
		}}

	repo := newTestRepo(content)

	loader := repo.loader
	repo.loader = func(ctx context.Context, packID restic.ID, blobs []restic.Blob, handleBlobFn func(blob restic.BlobHandle, buf []byte, err error) error) error {
		ctr := 0
		return loader(ctx, packID, blobs, func(blob restic.BlobHandle, buf []byte, err error) error {
			if ctr < 2 {
				ctr++
				return handleBlobFn(blob, buf, err)
			}
			// break file2
			return errors.New("failed to load blob")
		})
	}

	r := newFileRestorer(tempdir, repo.loader, repo.Lookup, 2, false, false, nil)
	r.files = repo.files

	var errors []string
	r.Error = func(s string, e error) error {
		// ignore errors as in the `restore` command
		errors = append(errors, s)
		return nil
	}

	err := r.restoreFiles(context.TODO())
	rtest.OK(t, err)

	rtest.Assert(t, len(errors) == 1, "unexpected number of restore errors, expected: 1, got: %v", len(errors))
	rtest.Assert(t, errors[0] == "file2", "expected error for file2, got: %v", errors[0])
}
