package repository_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/klauspost/compress/zstd"
	"github.com/restic/restic/internal/crypto"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/test"
	rtest "github.com/restic/restic/internal/test"
	"golang.org/x/sync/errgroup"
)

var testSizes = []int{5, 23, 2<<18 + 23, 1 << 20}

var rnd = rand.New(rand.NewSource(time.Now().UnixNano()))

func TestSave(t *testing.T) {
	repository.TestAllVersions(t, testSave)
}

func testSave(t *testing.T, version uint) {
	repo, cleanup := repository.TestRepositoryWithVersion(t, version)
	defer cleanup()

	for _, size := range testSizes {
		data := make([]byte, size)
		_, err := io.ReadFull(rnd, data)
		rtest.OK(t, err)

		id := restic.Hash(data)

		var wg errgroup.Group
		repo.StartPackUploader(context.TODO(), &wg)

		// save
		sid, _, _, err := repo.SaveBlob(context.TODO(), restic.DataBlob, data, restic.ID{}, false)
		rtest.OK(t, err)

		rtest.Equals(t, id, sid)

		rtest.OK(t, repo.Flush(context.Background()))
		// rtest.OK(t, repo.SaveIndex())

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

func TestSaveFrom(t *testing.T) {
	repository.TestAllVersions(t, testSaveFrom)
}

func testSaveFrom(t *testing.T, version uint) {
	repo, cleanup := repository.TestRepositoryWithVersion(t, version)
	defer cleanup()

	for _, size := range testSizes {
		data := make([]byte, size)
		_, err := io.ReadFull(rnd, data)
		rtest.OK(t, err)

		id := restic.Hash(data)

		var wg errgroup.Group
		repo.StartPackUploader(context.TODO(), &wg)

		// save
		id2, _, _, err := repo.SaveBlob(context.TODO(), restic.DataBlob, data, id, false)
		rtest.OK(t, err)
		rtest.Equals(t, id, id2)

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
	repo, cleanup := repository.TestRepositoryWithVersion(t, version)
	defer cleanup()

	size := 4 << 20 // 4MiB

	data := make([]byte, size)
	_, err := io.ReadFull(rnd, data)
	rtest.OK(t, err)

	id := restic.ID(sha256.Sum256(data))

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
	repo, cleanup := repository.TestRepositoryWithVersion(t, version)
	defer cleanup()

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
	repo, cleanup := repository.TestRepositoryWithVersion(b, version)
	defer cleanup()

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
	repo, cleanup := repository.TestRepositoryWithVersion(b, version)
	defer cleanup()

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
		data, err := repo.LoadUnpacked(context.TODO(), restic.PackFile, storageID, nil)
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
	repodir, cleanup := rtest.Env(t, repoFixture)
	defer cleanup()

	repo := repository.TestOpenLocal(t, repodir)
	rtest.OK(t, repo.LoadIndex(context.TODO()))
}

// loadIndex loads the index id from backend and returns it.
func loadIndex(ctx context.Context, repo restic.Repository, id restic.ID) (*repository.Index, error) {
	buf, err := repo.LoadUnpacked(ctx, restic.IndexFile, id, nil)
	if err != nil {
		return nil, err
	}

	idx, oldFormat, err := repository.DecodeIndex(buf, id)
	if oldFormat {
		fmt.Fprintf(os.Stderr, "index %v has old format\n", id.Str())
	}
	return idx, err
}

func BenchmarkLoadIndex(b *testing.B) {
	repository.BenchmarkAllVersions(b, benchmarkLoadIndex)
}

func benchmarkLoadIndex(b *testing.B, version uint) {
	repository.TestUseLowSecurityKDFParameters(b)

	repo, cleanup := repository.TestRepositoryWithVersion(b, version)
	defer cleanup()

	idx := repository.NewIndex()

	for i := 0; i < 5000; i++ {
		idx.StorePack(restic.NewRandomID(), []restic.Blob{
			{
				BlobHandle: restic.NewRandomBlobHandle(),
				Length:     1234,
				Offset:     1235,
			},
		})
	}

	id, err := repository.SaveIndex(context.TODO(), repo, idx)
	rtest.OK(b, err)

	b.Logf("index saved as %v", id.Str())
	fi, err := repo.Backend().Stat(context.TODO(), restic.Handle{Type: restic.IndexFile, Name: id.String()})
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
	r, cleanup := repository.TestRepositoryWithVersion(t, version)
	defer cleanup()

	repo := r.(*repository.Repository)

	repository.IndexFull = func(*repository.Index, bool) bool { return true }

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

		for pb := range idx.Each(context.TODO()) {
			if _, ok := packEntries[pb.PackID]; !ok {
				packEntries[pb.PackID] = make(map[restic.ID]struct{})
			}

			packEntries[pb.PackID][id] = struct{}{}
		}
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

// buildPackfileWithoutHeader returns a manually built pack file without a header.
func buildPackfileWithoutHeader(t testing.TB, blobSizes []int, key *crypto.Key, compress bool) (blobs []restic.Blob, packfile []byte) {
	opts := []zstd.EOption{
		// Set the compression level configured.
		zstd.WithEncoderLevel(zstd.SpeedDefault),
		// Disable CRC, we have enough checks in place, makes the
		// compressed data four bytes shorter.
		zstd.WithEncoderCRC(false),
		// Set a window of 512kbyte, so we have good lookbehind for usual
		// blob sizes.
		zstd.WithWindowSize(512 * 1024),
	}
	enc, err := zstd.NewWriter(nil, opts...)
	if err != nil {
		panic(err)
	}

	var offset uint
	for i, size := range blobSizes {
		plaintext := test.Random(800+i, size)
		id := restic.Hash(plaintext)
		uncompressedLength := uint(0)
		if compress {
			uncompressedLength = uint(len(plaintext))
			plaintext = enc.EncodeAll(plaintext, nil)
		}

		// we use a deterministic nonce here so the whole process is
		// deterministic, last byte is the blob index
		var nonce = []byte{
			0x15, 0x98, 0xc0, 0xf7, 0xb9, 0x65, 0x97, 0x74,
			0x12, 0xdc, 0xd3, 0x62, 0xa9, 0x6e, 0x20, byte(i),
		}

		before := len(packfile)
		packfile = append(packfile, nonce...)
		packfile = key.Seal(packfile, nonce, plaintext, nil)
		after := len(packfile)

		ciphertextLength := after - before

		blobs = append(blobs, restic.Blob{
			BlobHandle: restic.BlobHandle{
				Type: restic.DataBlob,
				ID:   id,
			},
			Length:             uint(ciphertextLength),
			UncompressedLength: uncompressedLength,
			Offset:             offset,
		})

		offset = uint(len(packfile))
	}

	return blobs, packfile
}

func TestStreamPack(t *testing.T) {
	repository.TestAllVersions(t, testStreamPack)
}

func testStreamPack(t *testing.T, version uint) {
	// always use the same key for deterministic output
	const jsonKey = `{"mac":{"k":"eQenuI8adktfzZMuC8rwdA==","r":"k8cfAly2qQSky48CQK7SBA=="},"encrypt":"MKO9gZnRiQFl8mDUurSDa9NMjiu9MUifUrODTHS05wo="}`

	var key crypto.Key
	err := json.Unmarshal([]byte(jsonKey), &key)
	if err != nil {
		t.Fatal(err)
	}

	blobSizes := []int{
		5522811,
		10,
		5231,
		18812,
		123123,
		13522811,
		12301,
		892242,
		28616,
		13351,
		252287,
		188883,
		3522811,
		18883,
	}

	var compress bool
	switch version {
	case 1:
		compress = false
	case 2:
		compress = true
	default:
		t.Fatal("test does not suport repository version", version)
	}

	packfileBlobs, packfile := buildPackfileWithoutHeader(t, blobSizes, &key, compress)

	loadCalls := 0
	load := func(ctx context.Context, h restic.Handle, length int, offset int64, fn func(rd io.Reader) error) error {
		data := packfile

		if offset > int64(len(data)) {
			offset = 0
			length = 0
		}
		data = data[offset:]

		if length > len(data) {
			length = len(data)
		}

		data = data[:length]
		loadCalls++

		return fn(bytes.NewReader(data))

	}

	// first, test regular usage
	t.Run("regular", func(t *testing.T) {
		tests := []struct {
			blobs []restic.Blob
			calls int
		}{
			{packfileBlobs[1:2], 1},
			{packfileBlobs[2:5], 1},
			{packfileBlobs[2:8], 1},
			{[]restic.Blob{
				packfileBlobs[0],
				packfileBlobs[4],
				packfileBlobs[2],
			}, 1},
			{[]restic.Blob{
				packfileBlobs[0],
				packfileBlobs[len(packfileBlobs)-1],
			}, 2},
		}

		for _, test := range tests {
			t.Run("", func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()

				gotBlobs := make(map[restic.ID]int)

				handleBlob := func(blob restic.BlobHandle, buf []byte, err error) error {
					gotBlobs[blob.ID]++

					id := restic.Hash(buf)
					if !id.Equal(blob.ID) {
						t.Fatalf("wrong id %v for blob %s returned", id, blob.ID)
					}

					return err
				}

				wantBlobs := make(map[restic.ID]int)
				for _, blob := range test.blobs {
					wantBlobs[blob.ID] = 1
				}

				loadCalls = 0
				err = repository.StreamPack(ctx, load, &key, restic.ID{}, test.blobs, handleBlob)
				if err != nil {
					t.Fatal(err)
				}

				if !cmp.Equal(wantBlobs, gotBlobs) {
					t.Fatal(cmp.Diff(wantBlobs, gotBlobs))
				}
				rtest.Equals(t, test.calls, loadCalls)
			})
		}
	})

	// next, test invalid uses, which should return an error
	t.Run("invalid", func(t *testing.T) {
		tests := []struct {
			blobs []restic.Blob
			err   string
		}{
			{
				// pass one blob several times
				blobs: []restic.Blob{
					packfileBlobs[3],
					packfileBlobs[8],
					packfileBlobs[3],
					packfileBlobs[4],
				},
				err: "overlapping blobs in pack",
			},

			{
				// pass something that's not a valid blob in the current pack file
				blobs: []restic.Blob{
					{
						Offset: 123,
						Length: 20000,
					},
				},
				err: "ciphertext verification failed",
			},

			{
				// pass a blob that's too small
				blobs: []restic.Blob{
					{
						Offset: 123,
						Length: 10,
					},
				},
				err: "invalid blob length",
			},
		}

		for _, test := range tests {
			t.Run("", func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()

				handleBlob := func(blob restic.BlobHandle, buf []byte, err error) error {
					return err
				}

				err = repository.StreamPack(ctx, load, &key, restic.ID{}, test.blobs, handleBlob)
				if err == nil {
					t.Fatalf("wanted error %v, got nil", test.err)
				}

				if !strings.Contains(err.Error(), test.err) {
					t.Fatalf("wrong error returned, it should contain %q but was %q", test.err, err)
				}
			})
		}
	})
}
