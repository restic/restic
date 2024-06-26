package repository_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/backend/cache"
	"github.com/restic/restic/internal/backend/local"
	"github.com/restic/restic/internal/backend/mem"
	"github.com/restic/restic/internal/crypto"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/repository/index"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/test"
	rtest "github.com/restic/restic/internal/test"
	"golang.org/x/sync/errgroup"
)

var testSizes = []int{5, 23, 2<<18 + 23, 1 << 20}

var rnd = rand.New(rand.NewSource(time.Now().UnixNano()))

func TestSave(t *testing.T) {
	repository.TestAllVersions(t, testSavePassID)
	repository.TestAllVersions(t, testSaveCalculateID)
}

func testSavePassID(t *testing.T, version uint) {
	testSave(t, version, false)
}

func testSaveCalculateID(t *testing.T, version uint) {
	testSave(t, version, true)
}

func testSave(t *testing.T, version uint, calculateID bool) {
	repo, _ := repository.TestRepositoryWithVersion(t, version)

	for _, size := range testSizes {
		data := make([]byte, size)
		_, err := io.ReadFull(rnd, data)
		rtest.OK(t, err)

		id := restic.Hash(data)

		var wg errgroup.Group
		repo.StartPackUploader(context.TODO(), &wg)

		// save
		inputID := restic.ID{}
		if !calculateID {
			inputID = id
		}
		sid, _, _, err := repo.SaveBlob(context.TODO(), restic.DataBlob, data, inputID, false)
		rtest.OK(t, err)
		rtest.Equals(t, id, sid)

		rtest.OK(t, repo.Flush(context.Background()))

		// read back
		buf, err := repo.LoadBlob(context.TODO(), restic.DataBlob, id, nil)
		rtest.OK(t, err)
		rtest.Equals(t, size, len(buf))

		rtest.Assert(t, len(buf) == len(data),
			"number of bytes read back does not match: expected %d, got %d",
			len(data), len(buf))

		rtest.Assert(t, bytes.Equal(buf, data),
			"data does not match: expected %02x, got %02x",
			data, buf)
	}
}

func BenchmarkSaveAndEncrypt(t *testing.B) {
	repository.BenchmarkAllVersions(t, benchmarkSaveAndEncrypt)
}

func benchmarkSaveAndEncrypt(t *testing.B, version uint) {
	repo, _ := repository.TestRepositoryWithVersion(t, version)
	size := 4 << 20 // 4MiB

	data := make([]byte, size)
	_, err := io.ReadFull(rnd, data)
	rtest.OK(t, err)

	id := restic.ID(sha256.Sum256(data))
	var wg errgroup.Group
	repo.StartPackUploader(context.Background(), &wg)

	t.ReportAllocs()
	t.ResetTimer()
	t.SetBytes(int64(size))

	for i := 0; i < t.N; i++ {
		_, _, _, err = repo.SaveBlob(context.TODO(), restic.DataBlob, data, id, true)
		rtest.OK(t, err)
	}
}

func TestLoadBlob(t *testing.T) {
	repository.TestAllVersions(t, testLoadBlob)
}

func testLoadBlob(t *testing.T, version uint) {
	repo, _ := repository.TestRepositoryWithVersion(t, version)
	length := 1000000
	buf := crypto.NewBlobBuffer(length)
	_, err := io.ReadFull(rnd, buf)
	rtest.OK(t, err)

	var wg errgroup.Group
	repo.StartPackUploader(context.TODO(), &wg)

	id, _, _, err := repo.SaveBlob(context.TODO(), restic.DataBlob, buf, restic.ID{}, false)
	rtest.OK(t, err)
	rtest.OK(t, repo.Flush(context.Background()))

	base := crypto.CiphertextLength(length)
	for _, testlength := range []int{0, base - 20, base - 1, base, base + 7, base + 15, base + 1000} {
		buf = make([]byte, 0, testlength)
		buf, err := repo.LoadBlob(context.TODO(), restic.DataBlob, id, buf)
		if err != nil {
			t.Errorf("LoadBlob() returned an error for buffer size %v: %v", testlength, err)
			continue
		}

		if len(buf) != length {
			t.Errorf("LoadBlob() returned the wrong number of bytes: want %v, got %v", length, len(buf))
			continue
		}
	}
}

func TestLoadBlobBroken(t *testing.T) {
	be := mem.New()
	repo, _ := repository.TestRepositoryWithBackend(t, &damageOnceBackend{Backend: be}, restic.StableRepoVersion, repository.Options{})
	buf := test.Random(42, 1000)

	var wg errgroup.Group
	repo.StartPackUploader(context.TODO(), &wg)
	id, _, _, err := repo.SaveBlob(context.TODO(), restic.TreeBlob, buf, restic.ID{}, false)
	rtest.OK(t, err)
	rtest.OK(t, repo.Flush(context.Background()))

	// setup cache after saving the blob to make sure that the damageOnceBackend damages the cached data
	c := cache.TestNewCache(t)
	repo.UseCache(c)

	data, err := repo.LoadBlob(context.TODO(), restic.TreeBlob, id, nil)
	rtest.OK(t, err)
	rtest.Assert(t, bytes.Equal(buf, data), "data mismatch")
	pack := repo.LookupBlob(restic.TreeBlob, id)[0].PackID
	rtest.Assert(t, c.Has(backend.Handle{Type: restic.PackFile, Name: pack.String()}), "expected tree pack to be cached")
}

func BenchmarkLoadBlob(b *testing.B) {
	repository.BenchmarkAllVersions(b, benchmarkLoadBlob)
}

func benchmarkLoadBlob(b *testing.B, version uint) {
	repo, _ := repository.TestRepositoryWithVersion(b, version)
	length := 1000000
	buf := crypto.NewBlobBuffer(length)
	_, err := io.ReadFull(rnd, buf)
	rtest.OK(b, err)

	var wg errgroup.Group
	repo.StartPackUploader(context.TODO(), &wg)

	id, _, _, err := repo.SaveBlob(context.TODO(), restic.DataBlob, buf, restic.ID{}, false)
	rtest.OK(b, err)
	rtest.OK(b, repo.Flush(context.Background()))

	b.ResetTimer()
	b.SetBytes(int64(length))

	for i := 0; i < b.N; i++ {
		var err error
		buf, err = repo.LoadBlob(context.TODO(), restic.DataBlob, id, buf)

		// Checking the SHA-256 with restic.Hash can make up 38% of the time
		// spent in this loop, so pause the timer.
		b.StopTimer()
		rtest.OK(b, err)
		if len(buf) != length {
			b.Errorf("wanted %d bytes, got %d", length, len(buf))
		}

		id2 := restic.Hash(buf)
		if !id.Equal(id2) {
			b.Errorf("wrong data returned, wanted %v, got %v", id.Str(), id2.Str())
		}
		b.StartTimer()
	}
}

func BenchmarkLoadUnpacked(b *testing.B) {
	repository.BenchmarkAllVersions(b, benchmarkLoadUnpacked)
}

func benchmarkLoadUnpacked(b *testing.B, version uint) {
	repo, _ := repository.TestRepositoryWithVersion(b, version)
	length := 1000000
	buf := crypto.NewBlobBuffer(length)
	_, err := io.ReadFull(rnd, buf)
	rtest.OK(b, err)

	dataID := restic.Hash(buf)

	storageID, err := repo.SaveUnpacked(context.TODO(), restic.PackFile, buf)
	rtest.OK(b, err)
	// rtest.OK(b, repo.Flush())

	b.ResetTimer()
	b.SetBytes(int64(length))

	for i := 0; i < b.N; i++ {
		data, err := repo.LoadUnpacked(context.TODO(), restic.PackFile, storageID)
		rtest.OK(b, err)

		// See comment in BenchmarkLoadBlob.
		b.StopTimer()
		if len(data) != length {
			b.Errorf("wanted %d bytes, got %d", length, len(data))
		}

		id2 := restic.Hash(data)
		if !dataID.Equal(id2) {
			b.Errorf("wrong data returned, wanted %v, got %v", storageID.Str(), id2.Str())
		}
		b.StartTimer()
	}
}

var repoFixture = filepath.Join("testdata", "test-repo.tar.gz")

func TestRepositoryLoadIndex(t *testing.T) {
	repo, _, cleanup := repository.TestFromFixture(t, repoFixture)
	defer cleanup()

	rtest.OK(t, repo.LoadIndex(context.TODO(), nil))
}

// loadIndex loads the index id from backend and returns it.
func loadIndex(ctx context.Context, repo restic.LoaderUnpacked, id restic.ID) (*index.Index, error) {
	buf, err := repo.LoadUnpacked(ctx, restic.IndexFile, id)
	if err != nil {
		return nil, err
	}

	idx, oldFormat, err := index.DecodeIndex(buf, id)
	if oldFormat {
		fmt.Fprintf(os.Stderr, "index %v has old format\n", id.Str())
	}
	return idx, err
}

func TestRepositoryLoadUnpackedBroken(t *testing.T) {
	repo, be := repository.TestRepositoryWithVersion(t, 0)

	data := rtest.Random(23, 12345)
	id := restic.Hash(data)
	h := backend.Handle{Type: restic.IndexFile, Name: id.String()}
	// damage buffer
	data[0] ^= 0xff

	// store broken file
	err := be.Save(context.TODO(), h, backend.NewByteReader(data, be.Hasher()))
	rtest.OK(t, err)

	_, err = repo.LoadUnpacked(context.TODO(), restic.IndexFile, id)
	rtest.Assert(t, errors.Is(err, restic.ErrInvalidData), "unexpected error: %v", err)
}

type damageOnceBackend struct {
	backend.Backend
	m sync.Map
}

func (be *damageOnceBackend) Load(ctx context.Context, h backend.Handle, length int, offset int64, fn func(rd io.Reader) error) error {
	// don't break the config file as we can't retry it
	if h.Type == restic.ConfigFile {
		return be.Backend.Load(ctx, h, length, offset, fn)
	}

	h.IsMetadata = false
	_, isRetry := be.m.LoadOrStore(h, true)
	if !isRetry {
		// return broken data on the first try
		offset++
	}
	return be.Backend.Load(ctx, h, length, offset, fn)
}

func TestRepositoryLoadUnpackedRetryBroken(t *testing.T) {
	repodir, cleanup := rtest.Env(t, repoFixture)
	defer cleanup()

	be, err := local.Open(context.TODO(), local.Config{Path: repodir, Connections: 2})
	rtest.OK(t, err)
	repo := repository.TestOpenBackend(t, &damageOnceBackend{Backend: be})

	rtest.OK(t, repo.LoadIndex(context.TODO(), nil))
}

func BenchmarkLoadIndex(b *testing.B) {
	repository.BenchmarkAllVersions(b, benchmarkLoadIndex)
}

func benchmarkLoadIndex(b *testing.B, version uint) {
	repository.TestUseLowSecurityKDFParameters(b)

	repo, be := repository.TestRepositoryWithVersion(b, version)
	idx := index.NewIndex()

	for i := 0; i < 5000; i++ {
		idx.StorePack(restic.NewRandomID(), []restic.Blob{
			{
				BlobHandle: restic.NewRandomBlobHandle(),
				Length:     1234,
				Offset:     1235,
			},
		})
	}
	idx.Finalize()

	id, err := idx.SaveIndex(context.TODO(), repo)
	rtest.OK(b, err)

	b.Logf("index saved as %v", id.Str())
	fi, err := be.Stat(context.TODO(), backend.Handle{Type: restic.IndexFile, Name: id.String()})
	rtest.OK(b, err)
	b.Logf("filesize is %v", fi.Size)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := loadIndex(context.TODO(), repo, id)
		rtest.OK(b, err)
	}
}

// saveRandomDataBlobs generates random data blobs and saves them to the repository.
func saveRandomDataBlobs(t testing.TB, repo restic.Repository, num int, sizeMax int) {
	var wg errgroup.Group
	repo.StartPackUploader(context.TODO(), &wg)

	for i := 0; i < num; i++ {
		size := rand.Int() % sizeMax

		buf := make([]byte, size)
		_, err := io.ReadFull(rnd, buf)
		rtest.OK(t, err)

		_, _, _, err = repo.SaveBlob(context.TODO(), restic.DataBlob, buf, restic.ID{}, false)
		rtest.OK(t, err)
	}
}

func TestRepositoryIncrementalIndex(t *testing.T) {
	repository.TestAllVersions(t, testRepositoryIncrementalIndex)
}

func testRepositoryIncrementalIndex(t *testing.T, version uint) {
	repo, _ := repository.TestRepositoryWithVersion(t, version)

	index.IndexFull = func(*index.Index) bool { return true }

	// add a few rounds of packs
	for j := 0; j < 5; j++ {
		// add some packs, write intermediate index
		saveRandomDataBlobs(t, repo, 20, 1<<15)
		rtest.OK(t, repo.Flush(context.TODO()))
	}

	// save final index
	rtest.OK(t, repo.Flush(context.TODO()))

	packEntries := make(map[restic.ID]map[restic.ID]struct{})

	err := repo.List(context.TODO(), restic.IndexFile, func(id restic.ID, size int64) error {
		idx, err := loadIndex(context.TODO(), repo, id)
		rtest.OK(t, err)

		rtest.OK(t, idx.Each(context.TODO(), func(pb restic.PackedBlob) {
			if _, ok := packEntries[pb.PackID]; !ok {
				packEntries[pb.PackID] = make(map[restic.ID]struct{})
			}

			packEntries[pb.PackID][id] = struct{}{}
		}))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	for packID, ids := range packEntries {
		if len(ids) > 1 {
			t.Errorf("pack %v listed in %d indexes\n", packID, len(ids))
		}
	}

}

func TestInvalidCompression(t *testing.T) {
	var comp repository.CompressionMode
	err := comp.Set("nope")
	rtest.Assert(t, err != nil, "missing error")
	_, err = repository.New(nil, repository.Options{Compression: comp})
	rtest.Assert(t, err != nil, "missing error")
}

func TestListPack(t *testing.T) {
	be := mem.New()
	repo, _ := repository.TestRepositoryWithBackend(t, &damageOnceBackend{Backend: be}, restic.StableRepoVersion, repository.Options{})
	buf := test.Random(42, 1000)

	var wg errgroup.Group
	repo.StartPackUploader(context.TODO(), &wg)
	id, _, _, err := repo.SaveBlob(context.TODO(), restic.TreeBlob, buf, restic.ID{}, false)
	rtest.OK(t, err)
	rtest.OK(t, repo.Flush(context.Background()))

	// setup cache after saving the blob to make sure that the damageOnceBackend damages the cached data
	c := cache.TestNewCache(t)
	repo.UseCache(c)

	// Forcibly cache pack file
	packID := repo.LookupBlob(restic.TreeBlob, id)[0].PackID
	rtest.OK(t, be.Load(context.TODO(), backend.Handle{Type: restic.PackFile, IsMetadata: true, Name: packID.String()}, 0, 0, func(rd io.Reader) error { return nil }))

	// Get size to list pack
	var size int64
	rtest.OK(t, repo.List(context.TODO(), restic.PackFile, func(id restic.ID, sz int64) error {
		if id == packID {
			size = sz
		}
		return nil
	}))

	blobs, _, err := repo.ListPack(context.TODO(), packID, size)
	rtest.OK(t, err)
	rtest.Assert(t, len(blobs) == 1 && blobs[0].ID == id, "unexpected blobs in pack: %v", blobs)

	rtest.Assert(t, !c.Has(backend.Handle{Type: restic.PackFile, Name: packID.String()}), "tree pack should no longer be cached as ListPack does not set IsMetadata in the backend.Handle")
}

func TestNoDoubleInit(t *testing.T) {
	r, be := repository.TestRepositoryWithVersion(t, restic.StableRepoVersion)

	repo, err := repository.New(be, repository.Options{})
	rtest.OK(t, err)

	pol := r.Config().ChunkerPolynomial
	err = repo.Init(context.TODO(), r.Config().Version, test.TestPassword, &pol)
	rtest.Assert(t, strings.Contains(err.Error(), "repository master key and config already initialized"), "expected config exist error, got %q", err)

	// must also prevent init if only keys exist
	rtest.OK(t, be.Remove(context.TODO(), backend.Handle{Type: backend.ConfigFile}))
	err = repo.Init(context.TODO(), r.Config().Version, test.TestPassword, &pol)
	rtest.Assert(t, strings.Contains(err.Error(), "repository already contains keys"), "expected already contains keys error, got %q", err)

	// must also prevent init if a snapshot exists and keys were deleted
	var data [32]byte
	hash := restic.Hash(data[:])
	rtest.OK(t, be.Save(context.TODO(), backend.Handle{Type: backend.SnapshotFile, Name: hash.String()}, backend.NewByteReader(data[:], be.Hasher())))
	rtest.OK(t, be.List(context.TODO(), restic.KeyFile, func(fi backend.FileInfo) error {
		return be.Remove(context.TODO(), backend.Handle{Type: restic.KeyFile, Name: fi.Name})
	}))
	err = repo.Init(context.TODO(), r.Config().Version, test.TestPassword, &pol)
	rtest.Assert(t, strings.Contains(err.Error(), "repository already contains snapshots"), "expected already contains snapshots error, got %q", err)
}
