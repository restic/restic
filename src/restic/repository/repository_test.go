package repository_test

import (
	"bytes"
	"crypto/sha256"
	"io"
	"math/rand"
	"path/filepath"
	"testing"
	"time"

	"restic"
	"restic/archiver"
	"restic/repository"
	. "restic/test"
)

var testSizes = []int{5, 23, 2<<18 + 23, 1 << 20}

var rnd = rand.New(rand.NewSource(time.Now().UnixNano()))

func TestSave(t *testing.T) {
	repo, cleanup := repository.TestRepository(t)
	defer cleanup()

	for _, size := range testSizes {
		data := make([]byte, size)
		_, err := io.ReadFull(rnd, data)
		OK(t, err)

		id := restic.Hash(data)

		// save
		sid, err := repo.SaveBlob(restic.DataBlob, data, restic.ID{})
		OK(t, err)

		Equals(t, id, sid)

		OK(t, repo.Flush())
		// OK(t, repo.SaveIndex())

		// read back
		buf := restic.NewBlobBuffer(size)
		n, err := repo.LoadBlob(restic.DataBlob, id, buf)
		OK(t, err)
		Equals(t, len(buf), n)

		Assert(t, len(buf) == len(data),
			"number of bytes read back does not match: expected %d, got %d",
			len(data), len(buf))

		Assert(t, bytes.Equal(buf, data),
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
		OK(t, err)

		id := restic.Hash(data)

		// save
		id2, err := repo.SaveBlob(restic.DataBlob, data, id)
		OK(t, err)
		Equals(t, id, id2)

		OK(t, repo.Flush())

		// read back
		buf := restic.NewBlobBuffer(size)
		n, err := repo.LoadBlob(restic.DataBlob, id, buf)
		OK(t, err)
		Equals(t, len(buf), n)

		Assert(t, len(buf) == len(data),
			"number of bytes read back does not match: expected %d, got %d",
			len(data), len(buf))

		Assert(t, bytes.Equal(buf, data),
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
	OK(t, err)

	id := restic.ID(sha256.Sum256(data))

	t.ResetTimer()
	t.SetBytes(int64(size))

	for i := 0; i < t.N; i++ {
		// save
		_, err = repo.SaveBlob(restic.DataBlob, data, id)
		OK(t, err)
	}
}

func TestLoadTree(t *testing.T) {
	repo, cleanup := repository.TestRepository(t)
	defer cleanup()

	if BenchArchiveDirectory == "" {
		t.Skip("benchdir not set, skipping")
	}

	// archive a few files
	sn := archiver.TestSnapshot(t, repo, BenchArchiveDirectory, nil)
	OK(t, repo.Flush())

	_, err := repo.LoadTree(*sn.Tree)
	OK(t, err)
}

func BenchmarkLoadTree(t *testing.B) {
	repo, cleanup := repository.TestRepository(t)
	defer cleanup()

	if BenchArchiveDirectory == "" {
		t.Skip("benchdir not set, skipping")
	}

	// archive a few files
	sn := archiver.TestSnapshot(t, repo, BenchArchiveDirectory, nil)
	OK(t, repo.Flush())

	t.ResetTimer()

	for i := 0; i < t.N; i++ {
		_, err := repo.LoadTree(*sn.Tree)
		OK(t, err)
	}
}

func TestLoadBlob(t *testing.T) {
	repo, cleanup := repository.TestRepository(t)
	defer cleanup()

	length := 1000000
	buf := restic.NewBlobBuffer(length)
	_, err := io.ReadFull(rnd, buf)
	OK(t, err)

	id, err := repo.SaveBlob(restic.DataBlob, buf, restic.ID{})
	OK(t, err)
	OK(t, repo.Flush())

	// first, test with buffers that are too small
	for _, testlength := range []int{length - 20, length, restic.CiphertextLength(length) - 1} {
		buf = make([]byte, 0, testlength)
		n, err := repo.LoadBlob(restic.DataBlob, id, buf)
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
		n, err := repo.LoadBlob(restic.DataBlob, id, buf)
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
	OK(b, err)

	id, err := repo.SaveBlob(restic.DataBlob, buf, restic.ID{})
	OK(b, err)
	OK(b, repo.Flush())

	b.ResetTimer()
	b.SetBytes(int64(length))

	for i := 0; i < b.N; i++ {
		n, err := repo.LoadBlob(restic.DataBlob, id, buf)
		OK(b, err)
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
	OK(b, err)

	dataID := restic.Hash(buf)

	storageID, err := repo.SaveUnpacked(restic.DataFile, buf)
	OK(b, err)
	// OK(b, repo.Flush())

	b.ResetTimer()
	b.SetBytes(int64(length))

	for i := 0; i < b.N; i++ {
		data, err := repo.LoadAndDecrypt(restic.DataFile, storageID)
		OK(b, err)
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

	if BenchArchiveDirectory == "" {
		t.Skip("benchdir not set, skipping")
	}

	// archive a snapshot
	sn := restic.Snapshot{}
	sn.Hostname = "foobar"
	sn.Username = "test!"

	id, err := repo.SaveJSONUnpacked(restic.SnapshotFile, &sn)
	OK(t, err)

	var sn2 restic.Snapshot

	// restore
	err = repo.LoadJSONUnpacked(restic.SnapshotFile, id, &sn2)
	OK(t, err)

	Equals(t, sn.Hostname, sn2.Hostname)
	Equals(t, sn.Username, sn2.Username)
}

var repoFixture = filepath.Join("testdata", "test-repo.tar.gz")

func TestRepositoryLoadIndex(t *testing.T) {
	repodir, cleanup := Env(t, repoFixture)
	defer cleanup()

	repo := repository.TestOpenLocal(t, repodir)
	OK(t, repo.LoadIndex())
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

	id, err := repository.SaveIndex(repo, idx)
	OK(b, err)

	b.Logf("index saved as %v (%v entries)", id.Str(), idx.Count(restic.DataBlob))
	fi, err := repo.Backend().Stat(restic.Handle{Type: restic.IndexFile, Name: id.String()})
	OK(b, err)
	b.Logf("filesize is %v", fi.Size)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := repository.LoadIndex(repo, id)
		OK(b, err)
	}
}

// saveRandomDataBlobs generates random data blobs and saves them to the repository.
func saveRandomDataBlobs(t testing.TB, repo restic.Repository, num int, sizeMax int) {
	for i := 0; i < num; i++ {
		size := rand.Int() % sizeMax

		buf := make([]byte, size)
		_, err := io.ReadFull(rnd, buf)
		OK(t, err)

		_, err = repo.SaveBlob(restic.DataBlob, buf, restic.ID{})
		OK(t, err)
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
			OK(t, repo.Flush())
		}

		OK(t, repo.SaveFullIndex())
	}

	// add another 5 packs
	for i := 0; i < 5; i++ {
		saveRandomDataBlobs(t, repo, 5, 1<<15)
		OK(t, repo.Flush())
	}

	// save final index
	OK(t, repo.SaveIndex())

	packEntries := make(map[restic.ID]map[restic.ID]struct{})

	for id := range repo.List(restic.IndexFile, nil) {
		idx, err := repository.LoadIndex(repo, id)
		OK(t, err)

		for pb := range idx.Each(nil) {
			if _, ok := packEntries[pb.PackID]; !ok {
				packEntries[pb.PackID] = make(map[restic.ID]struct{})
			}

			packEntries[pb.PackID][id] = struct{}{}
		}
	}

	for packID, ids := range packEntries {
		if len(ids) > 1 {
			t.Errorf("pack %v listed in %d indexes\n", packID, len(ids))
		}
	}
}
