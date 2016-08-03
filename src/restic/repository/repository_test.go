package repository_test

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"io"
	mrand "math/rand"
	"path/filepath"
	"testing"

	"restic"
	"restic/backend"
	"restic/pack"
	"restic/repository"
	. "restic/test"
)

type testJSONStruct struct {
	Foo uint32
	Bar string
	Baz []byte
}

var repoTests = []testJSONStruct{
	testJSONStruct{Foo: 23, Bar: "Teststring", Baz: []byte("xx")},
}

func TestSaveJSON(t *testing.T) {
	repo := SetupRepo()
	defer TeardownRepo(repo)

	for _, obj := range repoTests {
		data, err := json.Marshal(obj)
		OK(t, err)
		data = append(data, '\n')
		h := sha256.Sum256(data)

		id, err := repo.SaveJSON(pack.Tree, obj)
		OK(t, err)

		Assert(t, h == id,
			"TestSaveJSON: wrong plaintext ID: expected %02x, got %02x",
			h, id)
	}
}

func BenchmarkSaveJSON(t *testing.B) {
	repo := SetupRepo()
	defer TeardownRepo(repo)

	obj := repoTests[0]

	data, err := json.Marshal(obj)
	OK(t, err)
	data = append(data, '\n')
	h := sha256.Sum256(data)

	t.ResetTimer()

	for i := 0; i < t.N; i++ {
		id, err := repo.SaveJSON(pack.Tree, obj)
		OK(t, err)

		Assert(t, h == id,
			"TestSaveJSON: wrong plaintext ID: expected %02x, got %02x",
			h, id)
	}
}

var testSizes = []int{5, 23, 2<<18 + 23, 1 << 20}

func TestSave(t *testing.T) {
	repo := SetupRepo()
	defer TeardownRepo(repo)

	for _, size := range testSizes {
		data := make([]byte, size)
		_, err := io.ReadFull(rand.Reader, data)
		OK(t, err)

		id := backend.Hash(data)

		// save
		sid, err := repo.SaveAndEncrypt(pack.Data, data, nil)
		OK(t, err)

		Equals(t, id, sid)

		OK(t, repo.Flush())
		// OK(t, repo.SaveIndex())

		// read back
		buf, err := repo.LoadBlob(id, pack.Data, make([]byte, size))
		OK(t, err)

		Assert(t, len(buf) == len(data),
			"number of bytes read back does not match: expected %d, got %d",
			len(data), len(buf))

		Assert(t, bytes.Equal(buf, data),
			"data does not match: expected %02x, got %02x",
			data, buf)
	}
}

func TestSaveFrom(t *testing.T) {
	repo := SetupRepo()
	defer TeardownRepo(repo)

	for _, size := range testSizes {
		data := make([]byte, size)
		_, err := io.ReadFull(rand.Reader, data)
		OK(t, err)

		id := backend.Hash(data)

		// save
		id2, err := repo.SaveAndEncrypt(pack.Data, data, &id)
		OK(t, err)
		Equals(t, id, id2)

		OK(t, repo.Flush())

		// read back
		buf, err := repo.LoadBlob(id, pack.Data, make([]byte, size))
		OK(t, err)

		Assert(t, len(buf) == len(data),
			"number of bytes read back does not match: expected %d, got %d",
			len(data), len(buf))

		Assert(t, bytes.Equal(buf, data),
			"data does not match: expected %02x, got %02x",
			data, buf)
	}
}

func BenchmarkSaveAndEncrypt(t *testing.B) {
	repo := SetupRepo()
	defer TeardownRepo(repo)

	size := 4 << 20 // 4MiB

	data := make([]byte, size)
	_, err := io.ReadFull(rand.Reader, data)
	OK(t, err)

	id := backend.ID(sha256.Sum256(data))

	t.ResetTimer()
	t.SetBytes(int64(size))

	for i := 0; i < t.N; i++ {
		// save
		_, err = repo.SaveAndEncrypt(pack.Data, data, &id)
		OK(t, err)
	}
}

func TestLoadJSONPack(t *testing.T) {
	repo := SetupRepo()
	defer TeardownRepo(repo)

	if BenchArchiveDirectory == "" {
		t.Skip("benchdir not set, skipping")
	}

	// archive a few files
	sn := SnapshotDir(t, repo, BenchArchiveDirectory, nil)
	OK(t, repo.Flush())

	tree := restic.NewTree()
	err := repo.LoadJSONPack(pack.Tree, *sn.Tree, &tree)
	OK(t, err)
}

func BenchmarkLoadJSONPack(t *testing.B) {
	repo := SetupRepo()
	defer TeardownRepo(repo)

	if BenchArchiveDirectory == "" {
		t.Skip("benchdir not set, skipping")
	}

	// archive a few files
	sn := SnapshotDir(t, repo, BenchArchiveDirectory, nil)
	OK(t, repo.Flush())

	tree := restic.NewTree()

	t.ResetTimer()

	for i := 0; i < t.N; i++ {
		err := repo.LoadJSONPack(pack.Tree, *sn.Tree, &tree)
		OK(t, err)
	}
}

func TestLoadJSONUnpacked(t *testing.T) {
	repo := SetupRepo()
	defer TeardownRepo(repo)

	if BenchArchiveDirectory == "" {
		t.Skip("benchdir not set, skipping")
	}

	// archive a snapshot
	sn := restic.Snapshot{}
	sn.Hostname = "foobar"
	sn.Username = "test!"

	id, err := repo.SaveJSONUnpacked(backend.Snapshot, &sn)
	OK(t, err)

	var sn2 restic.Snapshot

	// restore
	err = repo.LoadJSONUnpacked(backend.Snapshot, id, &sn2)
	OK(t, err)

	Equals(t, sn.Hostname, sn2.Hostname)
	Equals(t, sn.Username, sn2.Username)
}

var repoFixture = filepath.Join("testdata", "test-repo.tar.gz")

func TestRepositoryLoadIndex(t *testing.T) {
	WithTestEnvironment(t, repoFixture, func(repodir string) {
		repo := OpenLocalRepo(t, repodir)
		OK(t, repo.LoadIndex())
	})
}

func BenchmarkLoadIndex(b *testing.B) {
	WithTestEnvironment(b, repoFixture, func(repodir string) {
		repo := OpenLocalRepo(b, repodir)
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			repo.SetIndex(repository.NewMasterIndex())
			OK(b, repo.LoadIndex())
		}
	})
}

// saveRandomDataBlobs generates random data blobs and saves them to the repository.
func saveRandomDataBlobs(t testing.TB, repo *repository.Repository, num int, sizeMax int) {
	for i := 0; i < num; i++ {
		size := mrand.Int() % sizeMax

		buf := make([]byte, size)
		_, err := io.ReadFull(rand.Reader, buf)
		OK(t, err)

		_, err = repo.SaveAndEncrypt(pack.Data, buf, nil)
		OK(t, err)
	}
}

func TestRepositoryIncrementalIndex(t *testing.T) {
	repo := SetupRepo()
	defer TeardownRepo(repo)

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

	type packEntry struct {
		id      backend.ID
		indexes []*repository.Index
	}

	packEntries := make(map[backend.ID]map[backend.ID]struct{})

	for id := range repo.List(backend.Index, nil) {
		idx, err := repository.LoadIndex(repo, id)
		OK(t, err)

		for pb := range idx.Each(nil) {
			if _, ok := packEntries[pb.PackID]; !ok {
				packEntries[pb.PackID] = make(map[backend.ID]struct{})
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
