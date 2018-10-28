package index

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/restic/restic/internal/checker"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/test"
)

var (
	snapshotTime = time.Unix(1470492820, 207401672)
	depth        = 3
)

func createFilledRepo(t testing.TB, snapshots int, dup float32) (restic.Repository, func()) {
	repo, cleanup := repository.TestRepository(t)

	for i := 0; i < 3; i++ {
		restic.TestCreateSnapshot(t, repo, snapshotTime.Add(time.Duration(i)*time.Second), depth, dup)
	}

	return repo, cleanup
}

func validateIndex(t testing.TB, repo restic.Repository, idx *Index) {
	err := repo.List(context.TODO(), restic.DataFile, func(id restic.ID, size int64) error {
		p, ok := idx.Packs[id]
		if !ok {
			t.Errorf("pack %v missing from index", id.Str())
		}

		if !p.ID.Equal(id) {
			t.Errorf("pack %v has invalid ID: want %v, got %v", id.Str(), id, p.ID)
		}
		return nil
	})

	if err != nil {
		t.Fatal(err)
	}
}

func TestIndexNew(t *testing.T) {
	repo, cleanup := createFilledRepo(t, 3, 0)
	defer cleanup()

	idx, invalid, err := New(context.TODO(), repo, restic.NewIDSet(), nil)
	if err != nil {
		t.Fatalf("New() returned error %v", err)
	}

	if idx == nil {
		t.Fatalf("New() returned nil index")
	}

	if len(invalid) > 0 {
		t.Fatalf("New() returned invalid files: %v", invalid)
	}

	validateIndex(t, repo, idx)
}

type ErrorRepo struct {
	restic.Repository
	MaxListFiles int

	MaxPacks      int
	MaxPacksMutex sync.Mutex
}

// List returns an error after repo.MaxListFiles files.
func (repo *ErrorRepo) List(ctx context.Context, t restic.FileType, fn func(restic.ID, int64) error) error {
	if repo.MaxListFiles == 0 {
		return errors.New("test error, max is zero")
	}

	max := repo.MaxListFiles
	return repo.Repository.List(ctx, t, func(id restic.ID, size int64) error {
		if max == 0 {
			return errors.New("test error, max reached zero")
		}

		max--
		return fn(id, size)
	})
}

// ListPack returns an error after repo.MaxPacks files.
func (repo *ErrorRepo) ListPack(ctx context.Context, id restic.ID, size int64) ([]restic.Blob, int64, error) {
	repo.MaxPacksMutex.Lock()
	max := repo.MaxPacks
	if max > 0 {
		repo.MaxPacks--
	}
	repo.MaxPacksMutex.Unlock()

	if max == 0 {
		return nil, 0, errors.New("test list pack error")
	}

	return repo.Repository.ListPack(ctx, id, size)
}

func TestIndexNewListErrors(t *testing.T) {
	repo, cleanup := createFilledRepo(t, 3, 0)
	defer cleanup()

	for _, max := range []int{0, 3, 5} {
		errRepo := &ErrorRepo{
			Repository:   repo,
			MaxListFiles: max,
		}
		idx, invalid, err := New(context.TODO(), errRepo, restic.NewIDSet(), nil)
		if err == nil {
			t.Errorf("expected error not found, got nil")
		}

		if idx != nil {
			t.Errorf("expected nil index, got %v", idx)
		}

		if len(invalid) != 0 {
			t.Errorf("expected empty invalid list, got %v", invalid)
		}
	}
}

func TestIndexNewPackErrors(t *testing.T) {
	repo, cleanup := createFilledRepo(t, 3, 0)
	defer cleanup()

	for _, max := range []int{0, 3, 5} {
		errRepo := &ErrorRepo{
			Repository: repo,
			MaxPacks:   max,
		}
		idx, invalid, err := New(context.TODO(), errRepo, restic.NewIDSet(), nil)
		if err == nil {
			t.Errorf("expected error not found, got nil")
		}

		if idx != nil {
			t.Errorf("expected nil index, got %v", idx)
		}

		if len(invalid) != 0 {
			t.Errorf("expected empty invalid list, got %v", invalid)
		}
	}
}

func TestIndexLoad(t *testing.T) {
	repo, cleanup := createFilledRepo(t, 3, 0)
	defer cleanup()

	loadIdx, err := Load(context.TODO(), repo, nil)
	if err != nil {
		t.Fatalf("Load() returned error %v", err)
	}

	if loadIdx == nil {
		t.Fatalf("Load() returned nil index")
	}

	validateIndex(t, repo, loadIdx)

	newIdx, _, err := New(context.TODO(), repo, restic.NewIDSet(), nil)
	if err != nil {
		t.Fatalf("New() returned error %v", err)
	}

	if len(loadIdx.Packs) != len(newIdx.Packs) {
		t.Errorf("number of packs does not match: want %v, got %v",
			len(loadIdx.Packs), len(newIdx.Packs))
	}

	validateIndex(t, repo, newIdx)

	for packID, packNew := range newIdx.Packs {
		packLoad, ok := loadIdx.Packs[packID]

		if !ok {
			t.Errorf("loaded index does not list pack %v", packID.Str())
			continue
		}

		if len(packNew.Entries) != len(packLoad.Entries) {
			t.Errorf("  number of entries in pack %v does not match: %d != %d\n  %v\n  %v",
				packID.Str(), len(packNew.Entries), len(packLoad.Entries),
				packNew.Entries, packLoad.Entries)
			continue
		}

		for _, entryNew := range packNew.Entries {
			found := false
			for _, entryLoad := range packLoad.Entries {
				if !entryLoad.ID.Equal(entryNew.ID) {
					continue
				}

				if entryLoad.Type != entryNew.Type {
					continue
				}

				if entryLoad.Offset != entryNew.Offset {
					continue
				}

				if entryLoad.Length != entryNew.Length {
					continue
				}

				found = true
				break
			}

			if !found {
				t.Errorf("blob not found in loaded index: %v", entryNew)
			}
		}
	}
}

func BenchmarkIndexNew(b *testing.B) {
	repo, cleanup := createFilledRepo(b, 3, 0)
	defer cleanup()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		idx, _, err := New(context.TODO(), repo, restic.NewIDSet(), nil)

		if err != nil {
			b.Fatalf("New() returned error %v", err)
		}

		if idx == nil {
			b.Fatalf("New() returned nil index")
		}
		b.Logf("idx %v packs", len(idx.Packs))
	}
}

func BenchmarkIndexSave(b *testing.B) {
	repo, cleanup := repository.TestRepository(b)
	defer cleanup()

	idx, _, err := New(context.TODO(), repo, restic.NewIDSet(), nil)
	test.OK(b, err)

	for i := 0; i < 8000; i++ {
		entries := make([]restic.Blob, 0, 200)
		for j := 0; j < cap(entries); j++ {
			entries = append(entries, restic.Blob{
				ID:     restic.NewRandomID(),
				Length: 1000,
				Offset: 5,
				Type:   restic.DataBlob,
			})
		}

		idx.AddPack(restic.NewRandomID(), 10000, entries)
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		ids, err := idx.Save(context.TODO(), repo, nil)
		if err != nil {
			b.Fatalf("New() returned error %v", err)
		}

		b.Logf("saved as %v", ids)
	}
}

func TestIndexDuplicateBlobs(t *testing.T) {
	repo, cleanup := createFilledRepo(t, 3, 0.05)
	defer cleanup()

	idx, _, err := New(context.TODO(), repo, restic.NewIDSet(), nil)
	if err != nil {
		t.Fatal(err)
	}

	dups := idx.DuplicateBlobs()
	if len(dups) == 0 {
		t.Errorf("no duplicate blobs found")
	}
	t.Logf("%d packs, %d duplicate blobs", len(idx.Packs), len(dups))

	packs := idx.PacksForBlobs(dups)
	if len(packs) == 0 {
		t.Errorf("no packs with duplicate blobs found")
	}
	t.Logf("%d packs with duplicate blobs", len(packs))
}

func loadIndex(t testing.TB, repo restic.Repository) *Index {
	idx, err := Load(context.TODO(), repo, nil)
	if err != nil {
		t.Fatalf("Load() returned error %v", err)
	}

	return idx
}

func TestIndexSave(t *testing.T) {
	repo, cleanup := createFilledRepo(t, 3, 0)
	defer cleanup()

	idx := loadIndex(t, repo)

	ids, err := idx.Save(context.TODO(), repo, idx.IndexIDs.List())
	if err != nil {
		t.Fatalf("unable to save new index: %v", err)
	}

	t.Logf("new index saved as %v", ids)

	for id := range idx.IndexIDs {
		t.Logf("remove index %v", id.Str())
		h := restic.Handle{Type: restic.IndexFile, Name: id.String()}
		err = repo.Backend().Remove(context.TODO(), h)
		if err != nil {
			t.Errorf("error removing index %v: %v", id, err)
		}
	}

	idx2 := loadIndex(t, repo)
	t.Logf("load new index with %d packs", len(idx2.Packs))

	checker := checker.New(repo)
	hints, errs := checker.LoadIndex(context.TODO())
	for _, h := range hints {
		t.Logf("hint: %v\n", h)
	}

	for _, err := range errs {
		t.Errorf("checker found error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	errCh := make(chan error)
	go checker.Structure(ctx, errCh)
	i := 0
	for err := range errCh {
		t.Errorf("checker returned error: %v", err)
		i++
		if i == 10 {
			t.Errorf("more than 10 errors returned, skipping the rest")
			cancel()
			break
		}
	}
}

func TestIndexAddRemovePack(t *testing.T) {
	repo, cleanup := createFilledRepo(t, 3, 0)
	defer cleanup()

	idx, err := Load(context.TODO(), repo, nil)
	if err != nil {
		t.Fatalf("Load() returned error %v", err)
	}

	var packID restic.ID
	err = repo.List(context.TODO(), restic.DataFile, func(id restic.ID, size int64) error {
		packID = id
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("selected pack %v", packID.Str())

	blobs := idx.Packs[packID].Entries

	idx.RemovePack(packID)

	if _, ok := idx.Packs[packID]; ok {
		t.Errorf("removed pack %v found in index.Packs", packID.Str())
	}

	for _, blob := range blobs {
		h := restic.BlobHandle{ID: blob.ID, Type: blob.Type}
		_, err := idx.FindBlob(h)
		if err == nil {
			t.Errorf("removed blob %v found in index", h)
		}
	}
}

// example index serialization from doc/Design.rst
var docExample = []byte(`
{
  "supersedes": [
	"ed54ae36197f4745ebc4b54d10e0f623eaaaedd03013eb7ae90df881b7781452"
  ],
  "packs": [
	{
	  "id": "73d04e6125cf3c28a299cc2f3cca3b78ceac396e4fcf9575e34536b26782413c",
	  "blobs": [
		{
		  "id": "3ec79977ef0cf5de7b08cd12b874cd0f62bbaf7f07f3497a5b1bbcc8cb39b1ce",
		  "type": "data",
		  "offset": 0,
		  "length": 25
		},{
		  "id": "9ccb846e60d90d4eb915848add7aa7ea1e4bbabfc60e573db9f7bfb2789afbae",
		  "type": "tree",
		  "offset": 38,
		  "length": 100
		},
		{
		  "id": "d3dc577b4ffd38cc4b32122cabf8655a0223ed22edfd93b353dc0c3f2b0fdf66",
		  "type": "data",
		  "offset": 150,
		  "length": 123
		}
	  ]
	}
  ]
}
`)

func TestIndexLoadDocReference(t *testing.T) {
	repo, cleanup := repository.TestRepository(t)
	defer cleanup()

	id, err := repo.SaveUnpacked(context.TODO(), restic.IndexFile, docExample)
	if err != nil {
		t.Fatalf("SaveUnpacked() returned error %v", err)
	}

	t.Logf("index saved as %v", id.Str())

	idx := loadIndex(t, repo)

	blobID := restic.TestParseID("d3dc577b4ffd38cc4b32122cabf8655a0223ed22edfd93b353dc0c3f2b0fdf66")
	locs, err := idx.FindBlob(restic.BlobHandle{ID: blobID, Type: restic.DataBlob})
	if err != nil {
		t.Errorf("FindBlob() returned error %v", err)
	}

	if len(locs) != 1 {
		t.Errorf("blob found %d times, expected just one", len(locs))
	}

	l := locs[0]
	if !l.ID.Equal(blobID) {
		t.Errorf("blob IDs are not equal: %v != %v", l.ID, blobID)
	}

	if l.Type != restic.DataBlob {
		t.Errorf("want type %v, got %v", restic.DataBlob, l.Type)
	}

	if l.Offset != 150 {
		t.Errorf("wrong offset, want %d, got %v", 150, l.Offset)
	}

	if l.Length != 123 {
		t.Errorf("wrong length, want %d, got %v", 123, l.Length)
	}
}
