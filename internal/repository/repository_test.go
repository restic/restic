package repository_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"io"
	"math/rand"
	"path/filepath"
	"testing"
	"time"

	"github.com/restic/restic/internal/archiver"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/fs"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
)

var testSizes = []int{5, 23, 2<<18 + 23, 1 << 20}

var rnd = rand.New(rand.NewSource(time.Now().UnixNano()))

func TestSave(t *testing.T) {
	repo, cleanup := repository.TestRepository(t)
	defer cleanup()

	for _, size := range testSizes {
		data := make([]byte, size)
		_, err := io.ReadFull(rnd, data)
		rtest.OK(t, err)

		id := restic.Hash(data)

		// save
		sid, err := repo.SaveBlob(context.TODO(), restic.DataBlob, data, restic.ID{})
		rtest.OK(t, err)

		rtest.Equals(t, id, sid)

		rtest.OK(t, repo.Flush(context.Background()))
		// rtest.OK(t, repo.SaveIndex())

		// read back
		buf := restic.NewBlobBuffer(size)
		n, err := repo.LoadBlob(context.TODO(), restic.DataBlob, id, buf)
		rtest.OK(t, err)
		rtest.Equals(t, len(buf), n)

		rtest.Assert(t, len(buf) == len(data),
			"number of bytes read back does not match: expected %d, got %d",
			len(data), len(buf))

		rtest.Assert(t, bytes.Equal(buf, data),
			"data does not match: expected %02x, got %02x",
			data, buf)
	}
}

func TestSaveFrom(t *testing.T) {
	repo, cleanup := repository.TestRepository(t)
	defer cleanup()

	for _, size := range testSizes {
		data := make([]byte, size)
		_, err := io.ReadFull(rnd, data)
		rtest.OK(t, err)

		id := restic.Hash(data)

		// save
		id2, err := repo.SaveBlob(context.TODO(), restic.DataBlob, data, id)
		rtest.OK(t, err)
		rtest.Equals(t, id, id2)

		rtest.OK(t, repo.Flush(context.Background()))

		// read back
		buf := restic.NewBlobBuffer(size)
		n, err := repo.LoadBlob(context.TODO(), restic.DataBlob, id, buf)
		rtest.OK(t, err)
		rtest.Equals(t, len(buf), n)

		rtest.Assert(t, len(buf) == len(data),
			"number of bytes read back does not match: expected %d, got %d",
			len(data), len(buf))

		rtest.Assert(t, bytes.Equal(buf, data),
			"data does not match: expected %02x, got %02x",
			data, buf)
	}
}

func BenchmarkSaveAndEncrypt(t *testing.B) {
	repo, cleanup := repository.TestRepository(t)
	defer cleanup()

	size := 4 << 20 // 4MiB

	data := make([]byte, size)
	_, err := io.ReadFull(rnd, data)
	rtest.OK(t, err)

	id := restic.ID(sha256.Sum256(data))

	t.ResetTimer()
	t.SetBytes(int64(size))

	for i := 0; i < t.N; i++ {
		// save
		_, err = repo.SaveBlob(context.TODO(), restic.DataBlob, data, id)
		rtest.OK(t, err)
	}
}

func TestLoadTree(t *testing.T) {
	repo, cleanup := repository.TestRepository(t)
	defer cleanup()

	if rtest.BenchArchiveDirectory == "" {
		t.Skip("benchdir not set, skipping")
	}

	// archive a few files
	sn := archiver.TestSnapshot(t, repo, rtest.BenchArchiveDirectory, nil)
	rtest.OK(t, repo.Flush(context.Background()))

	_, err := repo.LoadTree(context.TODO(), *sn.Tree)
	rtest.OK(t, err)
}

func BenchmarkLoadTree(t *testing.B) {
	repo, cleanup := repository.TestRepository(t)
	defer cleanup()

	if rtest.BenchArchiveDirectory == "" {
		t.Skip("benchdir not set, skipping")
	}

	// archive a few files
	sn := archiver.TestSnapshot(t, repo, rtest.BenchArchiveDirectory, nil)
	rtest.OK(t, repo.Flush(context.Background()))

	t.ResetTimer()

	for i := 0; i < t.N; i++ {
		_, err := repo.LoadTree(context.TODO(), *sn.Tree)
		rtest.OK(t, err)
	}
}

func TestLoadBlob(t *testing.T) {
	repo, cleanup := repository.TestRepository(t)
	defer cleanup()

	length := 1000000
	buf := restic.NewBlobBuffer(length)
	_, err := io.ReadFull(rnd, buf)
	rtest.OK(t, err)

	id, err := repo.SaveBlob(context.TODO(), restic.DataBlob, buf, restic.ID{})
	rtest.OK(t, err)
	rtest.OK(t, repo.Flush(context.Background()))

	// first, test with buffers that are too small
	for _, testlength := range []int{length - 20, length, restic.CiphertextLength(length) - 1} {
		buf = make([]byte, 0, testlength)
		n, err := repo.LoadBlob(context.TODO(), restic.DataBlob, id, buf)
		if err == nil {
			t.Errorf("LoadBlob() did not return an error for a buffer that is too small to hold the blob")
			continue
		}

		if n != 0 {
			t.Errorf("LoadBlob() returned an error and n > 0")
			continue
		}
	}

	// then use buffers that are large enough
	base := restic.CiphertextLength(length)
	for _, testlength := range []int{base, base + 7, base + 15, base + 1000} {
		buf = make([]byte, 0, testlength)
		n, err := repo.LoadBlob(context.TODO(), restic.DataBlob, id, buf)
		if err != nil {
			t.Errorf("LoadBlob() returned an error for buffer size %v: %v", testlength, err)
			continue
		}

		if n != length {
			t.Errorf("LoadBlob() returned the wrong number of bytes: want %v, got %v", length, n)
			continue
		}
	}
}

func BenchmarkLoadBlob(b *testing.B) {
	repo, cleanup := repository.TestRepository(b)
	defer cleanup()

	length := 1000000
	buf := restic.NewBlobBuffer(length)
	_, err := io.ReadFull(rnd, buf)
	rtest.OK(b, err)

	id, err := repo.SaveBlob(context.TODO(), restic.DataBlob, buf, restic.ID{})
	rtest.OK(b, err)
	rtest.OK(b, repo.Flush(context.Background()))

	b.ResetTimer()
	b.SetBytes(int64(length))

	for i := 0; i < b.N; i++ {
		n, err := repo.LoadBlob(context.TODO(), restic.DataBlob, id, buf)
		rtest.OK(b, err)
		if n != length {
			b.Errorf("wanted %d bytes, got %d", length, n)
		}

		id2 := restic.Hash(buf[:n])
		if !id.Equal(id2) {
			b.Errorf("wrong data returned, wanted %v, got %v", id.Str(), id2.Str())
		}
	}
}

func BenchmarkLoadAndDecrypt(b *testing.B) {
	repo, cleanup := repository.TestRepository(b)
	defer cleanup()

	length := 1000000
	buf := restic.NewBlobBuffer(length)
	_, err := io.ReadFull(rnd, buf)
	rtest.OK(b, err)

	dataID := restic.Hash(buf)

	storageID, err := repo.SaveUnpacked(context.TODO(), restic.DataFile, buf)
	rtest.OK(b, err)
	// rtest.OK(b, repo.Flush())

	b.ResetTimer()
	b.SetBytes(int64(length))

	for i := 0; i < b.N; i++ {
		data, err := repo.LoadAndDecrypt(context.TODO(), restic.DataFile, storageID)
		rtest.OK(b, err)
		if len(data) != length {
			b.Errorf("wanted %d bytes, got %d", length, len(data))
		}

		id2 := restic.Hash(data)
		if !dataID.Equal(id2) {
			b.Errorf("wrong data returned, wanted %v, got %v", storageID.Str(), id2.Str())
		}
	}
}

func TestLoadJSONUnpacked(t *testing.T) {
	repo, cleanup := repository.TestRepository(t)
	defer cleanup()

	if rtest.BenchArchiveDirectory == "" {
		t.Skip("benchdir not set, skipping")
	}

	// archive a snapshot
	sn := restic.Snapshot{}
	sn.Hostname = "foobar"
	sn.Username = "test!"

	id, err := repo.SaveJSONUnpacked(context.TODO(), restic.SnapshotFile, &sn)
	rtest.OK(t, err)

	var sn2 restic.Snapshot

	// restore
	err = repo.LoadJSONUnpacked(context.TODO(), restic.SnapshotFile, id, &sn2)
	rtest.OK(t, err)

	rtest.Equals(t, sn.Hostname, sn2.Hostname)
	rtest.Equals(t, sn.Username, sn2.Username)
}

var repoFixture = filepath.Join("testdata", "test-repo.tar.gz")

func TestRepositoryLoadIndex(t *testing.T) {
	repodir, cleanup := rtest.Env(t, repoFixture)
	defer cleanup()

	repo := repository.TestOpenLocal(t, repodir)
	rtest.OK(t, repo.LoadIndex(context.TODO()))
}

func BenchmarkLoadIndex(b *testing.B) {
	repository.TestUseLowSecurityKDFParameters(b)

	repo, cleanup := repository.TestRepository(b)
	defer cleanup()

	idx := repository.NewIndex()

	for i := 0; i < 5000; i++ {
		idx.Store(restic.PackedBlob{
			Blob: restic.Blob{
				Type:   restic.DataBlob,
				Length: 1234,
				ID:     restic.NewRandomID(),
				Offset: 1235,
			},
			PackID: restic.NewRandomID(),
		})
	}

	id, err := repository.SaveIndex(context.TODO(), repo, idx)
	rtest.OK(b, err)

	b.Logf("index saved as %v (%v entries)", id.Str(), idx.Count(restic.DataBlob))
	fi, err := repo.Backend().Stat(context.TODO(), restic.Handle{Type: restic.IndexFile, Name: id.String()})
	rtest.OK(b, err)
	b.Logf("filesize is %v", fi.Size)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := repository.LoadIndex(context.TODO(), repo, id)
		rtest.OK(b, err)
	}
}

// saveRandomDataBlobs generates random data blobs and saves them to the repository.
func saveRandomDataBlobs(t testing.TB, repo restic.Repository, num int, sizeMax int) {
	for i := 0; i < num; i++ {
		size := rand.Int() % sizeMax

		buf := make([]byte, size)
		_, err := io.ReadFull(rnd, buf)
		rtest.OK(t, err)

		_, err = repo.SaveBlob(context.TODO(), restic.DataBlob, buf, restic.ID{})
		rtest.OK(t, err)
	}
}

func TestRepositoryIncrementalIndex(t *testing.T) {
	repo, cleanup := repository.TestRepository(t)
	defer cleanup()

	repository.IndexFull = func(*repository.Index) bool { return true }

	// add 15 packs
	for j := 0; j < 5; j++ {
		// add 3 packs, write intermediate index
		for i := 0; i < 3; i++ {
			saveRandomDataBlobs(t, repo, 5, 1<<15)
			rtest.OK(t, repo.Flush(context.Background()))
		}

		rtest.OK(t, repo.SaveFullIndex(context.TODO()))
	}

	// add another 5 packs
	for i := 0; i < 5; i++ {
		saveRandomDataBlobs(t, repo, 5, 1<<15)
		rtest.OK(t, repo.Flush(context.Background()))
	}

	// save final index
	rtest.OK(t, repo.SaveIndex(context.TODO()))

	packEntries := make(map[restic.ID]map[restic.ID]struct{})

	err := repo.List(context.TODO(), restic.IndexFile, func(id restic.ID, size int64) error {
		idx, err := repository.LoadIndex(context.TODO(), repo, id)
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

type backend struct {
	rd io.Reader
}

func (be backend) Load(ctx context.Context, h restic.Handle, length int, offset int64, fn func(rd io.Reader) error) error {
	return fn(be.rd)
}

type retryBackend struct {
	buf []byte
}

func (be retryBackend) Load(ctx context.Context, h restic.Handle, length int, offset int64, fn func(rd io.Reader) error) error {
	err := fn(bytes.NewReader(be.buf[:len(be.buf)/2]))
	if err != nil {
		return err
	}

	return fn(bytes.NewReader(be.buf))
}

func TestDownloadAndHash(t *testing.T) {
	buf := make([]byte, 5*1024*1024+881)
	_, err := io.ReadFull(rnd, buf)
	if err != nil {
		t.Fatal(err)
	}

	var tests = []struct {
		be   repository.Loader
		want []byte
	}{
		{
			be:   backend{rd: bytes.NewReader(buf)},
			want: buf,
		},
		{
			be:   retryBackend{buf: buf},
			want: buf,
		},
	}

	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			f, id, size, err := repository.DownloadAndHash(context.TODO(), test.be, restic.Handle{})
			if err != nil {
				t.Error(err)
			}

			want := restic.Hash(test.want)
			if !want.Equal(id) {
				t.Errorf("wrong hash returned, want %v, got %v", want.Str(), id.Str())
			}

			if size != int64(len(test.want)) {
				t.Errorf("wrong size returned, want %v, got %v", test.want, size)
			}

			err = f.Close()
			if err != nil {
				t.Error(err)
			}

			err = fs.RemoveIfExists(f.Name())
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

type errorReader struct {
	err error
}

func (er errorReader) Read(p []byte) (n int, err error) {
	return 0, er.err
}

func TestDownloadAndHashErrors(t *testing.T) {
	var tests = []struct {
		be  repository.Loader
		err string
	}{
		{
			be:  backend{rd: errorReader{errors.New("test error 1")}},
			err: "test error 1",
		},
	}

	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			_, _, _, err := repository.DownloadAndHash(context.TODO(), test.be, restic.Handle{})
			if err == nil {
				t.Fatalf("wanted error %q, got nil", test.err)
			}

			if errors.Cause(err).Error() != test.err {
				t.Fatalf("wanted error %q, got %q", test.err, err)
			}
		})
	}
}
