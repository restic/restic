package restorer

import (
	"bytes"
	"cmp"
	"context"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/feature"
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

type TestWarmupJob struct {
	handlesCount int
	waitCalled   bool
}

type testPackBlob struct {
	packID     restic.ID
	handle     restic.BlobHandle
	offset     uint
	ciphertext uint
	plaintext  uint
	compressed bool
}

var _ restic.PackBlob = (*testPackBlob)(nil)

func (pb *testPackBlob) PackID() restic.ID { return pb.packID }

func (pb *testPackBlob) Handle() restic.BlobHandle { return pb.handle }

func (pb *testPackBlob) CiphertextLength() uint { return pb.ciphertext }

func (pb *testPackBlob) UncompressedCiphertextLength() uint { return pb.ciphertext }

func (pb *testPackBlob) PlaintextLength() uint { return pb.plaintext }

func (pb *testPackBlob) IsCompressed() bool { return pb.compressed }

type TestRepo struct {
	packsIDToData map[restic.ID][]byte

	// blobs and files
	blobs              map[restic.ID][]restic.PackBlob
	files              []*fileInfo
	filesPathToContent map[string]string

	warmupJobs []*TestWarmupJob

	//
	loader blobsLoaderFn
}

func (i *TestRepo) Lookup(bh restic.BlobHandle) []restic.PackBlob {
	packs := i.blobs[bh.ID]
	return packs
}

func (i *TestRepo) fileContent(file *fileInfo) string {
	return i.filesPathToContent[file.location]
}

func (i *TestRepo) StartWarmup(_ context.Context, packs restic.IDSet) (restic.WarmupJob, error) {
	job := TestWarmupJob{handlesCount: len(packs)}
	i.warmupJobs = append(i.warmupJobs, &job)
	return &job, nil
}

func (job *TestWarmupJob) HandleCount() int {
	return job.handlesCount
}

func (job *TestWarmupJob) Wait(_ context.Context) error {
	job.waitCalled = true
	return nil
}

func newTestRepo(content []TestFile) *TestRepo {
	type packBlobLayout struct {
		offset     uint
		ciphertext uint
		plaintext  uint
		compressed bool
	}
	type Pack struct {
		name  string
		data  []byte
		blobs map[restic.ID]packBlobLayout
	}
	packs := make(map[string]Pack)
	filesPathToContent := make(map[string]string)

	for _, file := range content {
		content := strings.Builder{}
		for _, blob := range file.blobs {
			content.WriteString(blob.data)

			// get the pack, create as necessary
			var pack Pack
			var found bool
			if pack, found = packs[blob.pack]; !found {
				pack = Pack{name: blob.pack, blobs: make(map[restic.ID]packBlobLayout)}
			}

			// calculate blob id and add to the pack as necessary
			blobID := restic.Hash([]byte(blob.data))
			if _, found := pack.blobs[blobID]; !found {
				blobData := []byte(blob.data)
				n := uint(len(blobData))
				pack.blobs[blobID] = packBlobLayout{
					offset:     uint(len(pack.data)),
					ciphertext: n,
					plaintext:  n,
					compressed: true,
				}
				pack.data = append(pack.data, blobData...)
			}

			packs[blob.pack] = pack
		}
		filesPathToContent[file.name] = content.String()
	}

	blobs := make(map[restic.ID][]restic.PackBlob)
	packsIDToData := make(map[restic.ID][]byte)

	for _, pack := range packs {
		packID := restic.Hash(pack.data)
		packsIDToData[packID] = pack.data
		for blobID, layout := range pack.blobs {
			blobs[blobID] = append(blobs[blobID], &testPackBlob{
				packID: packID,
				handle: restic.BlobHandle{Type: restic.DataBlob, ID: blobID},
				offset: layout.offset, ciphertext: layout.ciphertext,
				plaintext: layout.plaintext, compressed: layout.compressed,
			})
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
		warmupJobs:         []*TestWarmupJob{},
	}
	repo.loader = func(ctx context.Context, packID restic.ID, handles []restic.BlobHandle, handleBlobFn func(blob restic.BlobHandle, buf []byte, err error) error) error {
		entries := make([]*testPackBlob, 0, len(handles))
		for _, h := range handles {
			found := false
			for _, e := range repo.blobs[h.ID] {
				if packID == e.PackID() {
					entries = append(entries, e.(*testPackBlob))
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("missing blob: %v", h)
			}
		}
		slices.SortFunc(entries, func(a, b *testPackBlob) int {
			return cmp.Compare(a.offset, b.offset)
		})

		for _, e := range entries {
			buf := repo.packsIDToData[packID][e.offset : e.offset+e.ciphertext]
			err := handleBlobFn(e.handle, buf, nil)
			if err != nil {
				return err
			}
		}
		return nil
	}

	return repo
}

func restoreAndVerify(t *testing.T, tempdir string, content []TestFile, files map[string]bool, sparse bool) {
	defer feature.TestSetFlag(t, feature.Flag, feature.S3Restore, true)()

	t.Helper()
	repo := newTestRepo(content)

	r := newFileRestorer(tempdir, repo.loader, repo.Lookup, 2, sparse, false, repo.StartWarmup, nil,
		repository.TestRepository(t).ChunkerFactory().ZeroChunk())

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

	if len(repo.warmupJobs) == 0 {
		t.Errorf("warmup did not occur")
	}
	for i, warmupJob := range repo.warmupJobs {
		if !warmupJob.waitCalled {
			t.Errorf("warmup job %d was not waited", i)
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
		for range 10000 {
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
	repo.loader = func(ctx context.Context, packID restic.ID, handles []restic.BlobHandle, handleBlobFn func(blob restic.BlobHandle, buf []byte, err error) error) error {
		return loadError
	}

	r := newFileRestorer(tempdir, repo.loader, repo.Lookup, 2, false, false, repo.StartWarmup, nil,
		repository.TestRepository(t).ChunkerFactory().ZeroChunk())
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
	repo.loader = func(ctx context.Context, packID restic.ID, handles []restic.BlobHandle, handleBlobFn func(blob restic.BlobHandle, buf []byte, err error) error) error {
		ctr := 0
		return loader(ctx, packID, handles, func(blob restic.BlobHandle, buf []byte, err error) error {
			if ctr < 2 {
				ctr++
				return handleBlobFn(blob, buf, err)
			}
			// break file2
			return errors.New("failed to load blob")
		})
	}

	r := newFileRestorer(tempdir, repo.loader, repo.Lookup, 2, false, false, repo.StartWarmup, nil,
		repository.TestRepository(t).ChunkerFactory().ZeroChunk())
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
