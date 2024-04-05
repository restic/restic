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
	"testing"
	"time"

	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/backend/local"
	"github.com/restic/restic/internal/crypto"
	"github.com/restic/restic/internal/index"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
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
	repo := repository.TestRepositoryWithVersion(t, version)

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
	repo := repository.TestRepositoryWithVersion(t, version)
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
	repo := repository.TestRepositoryWithVersion(t, version)
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

func BenchmarkLoadBlob(b *testing.B) {
	repository.BenchmarkAllVersions(b, benchmarkLoadBlob)
}

func benchmarkLoadBlob(b *testing.B, version uint) {
	repo := repository.TestRepositoryWithVersion(b, version)
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
	repo := repository.TestRepositoryWithVersion(b, version)
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
	repo, cleanup := repository.TestFromFixture(t, repoFixture)
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
	repo := repository.TestRepository(t)

	data := rtest.Random(23, 12345)
	id := restic.Hash(data)
	h := backend.Handle{Type: restic.IndexFile, Name: id.String()}
	// damage buffer
	data[0] ^= 0xff

	// store broken file
	err := repo.Backend().Save(context.TODO(), h, backend.NewByteReader(data, repo.Backend().Hasher()))
	rtest.OK(t, err)

	// without a retry backend this will just return an error that the file is broken
	_, err = repo.LoadUnpacked(context.TODO(), restic.IndexFile, id)
	if err == nil {
		t.Fatal("missing expected error")
	}
	rtest.Assert(t, strings.Contains(err.Error(), "invalid data returned"), "unexpected error: %v", err)
}

type damageOnceBackend struct {
	backend.Backend
}

func (be *damageOnceBackend) Load(ctx context.Context, h backend.Handle, length int, offset int64, fn func(rd io.Reader) error) error {
	// don't break the config file as we can't retry it
	if h.Type == restic.ConfigFile {
		return be.Backend.Load(ctx, h, length, offset, fn)
	}
	// return broken data on the first try
	err := be.Backend.Load(ctx, h, length+1, offset, fn)
	if err != nil {
		// retry
		err = be.Backend.Load(ctx, h, length, offset, fn)
	}
	return err
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

	repo := repository.TestRepositoryWithVersion(b, version)
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

	id, err := index.SaveIndex(context.TODO(), repo, idx)
	rtest.OK(b, err)

	b.Logf("index saved as %v", id.Str())
	fi, err := repo.Backend().Stat(context.TODO(), backend.Handle{Type: restic.IndexFile, Name: id.String()})
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
	repo := repository.TestRepositoryWithVersion(t, version).(*repository.Repository)

	index.IndexFull = func(*index.Index, bool) bool { return true }

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
