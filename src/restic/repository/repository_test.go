package repository_test

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"io"
	mrand "math/rand"
	"path/filepath"
	"testing"

	"restic"
	"restic/archiver"
	"restic/repository"
	. "restic/test"
)

var testSizes = []int{5, 23, 2<<18 + 23, 1 << 20}

func TestSave(t *testing.T) {
	repo, cleanup := repository.TestRepository(t)
	defer cleanup()

	for _, size := range testSizes {
		data := make([]byte, size)
		_, err := io.ReadFull(rand.Reader, data)
		OK(t, err)

		id := restic.Hash(data)

		// save
		sid, err := repo.SaveBlob(restic.DataBlob, data, restic.ID{})
		OK(t, err)

		Equals(t, id, sid)

		OK(t, repo.Flush())
		// OK(t, repo.SaveIndex())

		// read back
		buf := make([]byte, size)
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
		_, err := io.ReadFull(rand.Reader, data)
		OK(t, err)

		id := restic.Hash(data)

		// save
		id2, err := repo.SaveBlob(restic.DataBlob, data, id)
		OK(t, err)
		Equals(t, id, id2)

		OK(t, repo.Flush())

		// read back
		buf := make([]byte, size)
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
	_, err := io.ReadFull(rand.Reader, data)
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
	repodir, cleanup := Env(b, repoFixture)
	defer cleanup()

	repo := repository.TestOpenLocal(b, repodir)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		repo.SetIndex(repository.NewMasterIndex())
		OK(b, repo.LoadIndex())
	}
}

// saveRandomDataBlobs generates random data blobs and saves them to the repository.
func saveRandomDataBlobs(t testing.TB, repo restic.Repository, num int, sizeMax int) {
	for i := 0; i < num; i++ {
		size := mrand.Int() % sizeMax

		buf := make([]byte, size)
		_, err := io.ReadFull(rand.Reader, buf)
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
