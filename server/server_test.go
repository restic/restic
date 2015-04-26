package server_test

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"flag"
	"io"
	"testing"

	"github.com/restic/restic"
	"github.com/restic/restic/backend"
	"github.com/restic/restic/pack"
	. "github.com/restic/restic/test"
)

var benchTestDir = flag.String("test.dir", ".", "dir used in benchmarks (default: .)")

type testJSONStruct struct {
	Foo uint32
	Bar string
	Baz []byte
}

var serverTests = []testJSONStruct{
	testJSONStruct{Foo: 23, Bar: "Teststring", Baz: []byte("xx")},
}

func TestSaveJSON(t *testing.T) {
	server := SetupBackend(t)
	defer TeardownBackend(t, server)
	key := SetupKey(t, server, "geheim")
	server.SetKey(key)

	for _, obj := range serverTests {
		data, err := json.Marshal(obj)
		OK(t, err)
		data = append(data, '\n')
		h := sha256.Sum256(data)

		id, err := server.SaveJSON(pack.Tree, obj)
		OK(t, err)

		Assert(t, bytes.Equal(h[:], id),
			"TestSaveJSON: wrong plaintext ID: expected %02x, got %02x",
			h, id)
	}
}

func BenchmarkSaveJSON(t *testing.B) {
	server := SetupBackend(t)
	defer TeardownBackend(t, server)
	key := SetupKey(t, server, "geheim")
	server.SetKey(key)

	obj := serverTests[0]

	data, err := json.Marshal(obj)
	OK(t, err)
	data = append(data, '\n')
	h := sha256.Sum256(data)

	t.ResetTimer()

	for i := 0; i < t.N; i++ {
		id, err := server.SaveJSON(pack.Tree, obj)
		OK(t, err)

		Assert(t, bytes.Equal(h[:], id),
			"TestSaveJSON: wrong plaintext ID: expected %02x, got %02x",
			h, id)
	}
}

var testSizes = []int{5, 23, 2<<18 + 23, 1 << 20}

func TestSave(t *testing.T) {
	server := SetupBackend(t)
	defer TeardownBackend(t, server)
	key := SetupKey(t, server, "geheim")
	server.SetKey(key)

	for _, size := range testSizes {
		data := make([]byte, size)
		_, err := io.ReadFull(rand.Reader, data)
		OK(t, err)

		id := backend.Hash(data)

		// save
		sid, err := server.Save(pack.Data, data, nil)
		OK(t, err)

		Equals(t, id, sid)

		OK(t, server.Flush())

		// read back
		buf, err := server.LoadBlob(pack.Data, id)

		Assert(t, len(buf) == len(data),
			"number of bytes read back does not match: expected %d, got %d",
			len(data), len(buf))

		Assert(t, bytes.Equal(buf, data),
			"data does not match: expected %02x, got %02x",
			data, buf)
	}
}

func TestSaveFrom(t *testing.T) {
	server := SetupBackend(t)
	defer TeardownBackend(t, server)
	key := SetupKey(t, server, "geheim")
	server.SetKey(key)

	for _, size := range testSizes {
		data := make([]byte, size)
		_, err := io.ReadFull(rand.Reader, data)
		OK(t, err)

		id := backend.Hash(data)

		// save
		err = server.SaveFrom(pack.Data, id[:], uint(size), bytes.NewReader(data))
		OK(t, err)

		OK(t, server.Flush())

		// read back
		buf, err := server.LoadBlob(pack.Data, id[:])

		Assert(t, len(buf) == len(data),
			"number of bytes read back does not match: expected %d, got %d",
			len(data), len(buf))

		Assert(t, bytes.Equal(buf, data),
			"data does not match: expected %02x, got %02x",
			data, buf)
	}
}

func BenchmarkSaveFrom(t *testing.B) {
	server := SetupBackend(t)
	defer TeardownBackend(t, server)
	key := SetupKey(t, server, "geheim")
	server.SetKey(key)

	size := 4 << 20 // 4MiB

	data := make([]byte, size)
	_, err := io.ReadFull(rand.Reader, data)
	OK(t, err)

	id := sha256.Sum256(data)

	t.ResetTimer()
	t.SetBytes(int64(size))

	for i := 0; i < t.N; i++ {
		// save
		err = server.SaveFrom(pack.Data, id[:], uint(size), bytes.NewReader(data))
		OK(t, err)
	}
}

func TestLoadJSONPack(t *testing.T) {
	if *benchTestDir == "" {
		t.Skip("benchdir not set, skipping TestServerStats")
	}

	server := SetupBackend(t)
	defer TeardownBackend(t, server)
	key := SetupKey(t, server, "geheim")
	server.SetKey(key)

	// archive a few files
	sn := SnapshotDir(t, server, *benchTestDir, nil)
	OK(t, server.Flush())

	tree := restic.NewTree()
	err := server.LoadJSONPack(pack.Tree, sn.Tree, &tree)
	OK(t, err)
}

func TestLoadJSONEncrypted(t *testing.T) {
	if *benchTestDir == "" {
		t.Skip("benchdir not set, skipping TestServerStats")
	}

	server := SetupBackend(t)
	defer TeardownBackend(t, server)
	key := SetupKey(t, server, "geheim")
	server.SetKey(key)

	// archive a snapshot
	sn := restic.Snapshot{}
	sn.Hostname = "foobar"
	sn.Username = "test!"

	id, err := server.SaveJSONUnpacked(backend.Snapshot, &sn)
	OK(t, err)

	var sn2 restic.Snapshot

	// restore
	err = server.LoadJSONEncrypted(backend.Snapshot, id, &sn2)
	OK(t, err)

	Equals(t, sn.Hostname, sn2.Hostname)
	Equals(t, sn.Username, sn2.Username)
}
