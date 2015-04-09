package restic_test

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"io"
	"testing"

	"github.com/restic/restic"
	"github.com/restic/restic/backend"
	. "github.com/restic/restic/test"
)

type testJSONStruct struct {
	Foo uint32
	Bar string
	Baz []byte
}

var serverTests = []testJSONStruct{
	testJSONStruct{Foo: 23, Bar: "Teststring", Baz: []byte("xx")},
}

func TestSaveJSON(t *testing.T) {
	server := setupBackend(t)
	defer teardownBackend(t, server)
	key := setupKey(t, server, "geheim")
	server.SetKey(key)

	for _, obj := range serverTests {
		data, err := json.Marshal(obj)
		OK(t, err)
		data = append(data, '\n')
		h := sha256.Sum256(data)

		blob, err := server.SaveJSON(backend.Tree, obj)
		OK(t, err)

		Assert(t, bytes.Equal(h[:], blob.ID),
			"TestSaveJSON: wrong plaintext ID: expected %02x, got %02x",
			h, blob.ID)
	}
}

func BenchmarkSaveJSON(t *testing.B) {
	server := setupBackend(t)
	defer teardownBackend(t, server)
	key := setupKey(t, server, "geheim")
	server.SetKey(key)

	obj := serverTests[0]

	data, err := json.Marshal(obj)
	OK(t, err)
	data = append(data, '\n')
	h := sha256.Sum256(data)

	t.ResetTimer()

	for i := 0; i < t.N; i++ {
		blob, err := server.SaveJSON(backend.Tree, obj)
		OK(t, err)

		Assert(t, bytes.Equal(h[:], blob.ID),
			"TestSaveJSON: wrong plaintext ID: expected %02x, got %02x",
			h, blob.ID)
	}
}

var testSizes = []int{5, 23, 2<<18 + 23, 1 << 20}

func TestSaveFrom(t *testing.T) {
	server := setupBackend(t)
	defer teardownBackend(t, server)
	key := setupKey(t, server, "geheim")
	server.SetKey(key)

	for _, size := range testSizes {
		data := make([]byte, size)
		_, err := io.ReadFull(rand.Reader, data)
		OK(t, err)

		id := sha256.Sum256(data)

		// save
		blob, err := server.SaveFrom(backend.Data, id[:], uint(size), bytes.NewReader(data))
		OK(t, err)

		// read back
		buf, err := server.Load(backend.Data, blob)

		Assert(t, len(buf) == len(data),
			"number of bytes read back does not match: expected %d, got %d",
			len(data), len(buf))

		Assert(t, bytes.Equal(buf, data),
			"data does not match: expected %02x, got %02x",
			data, buf)
	}
}

func BenchmarkSaveFrom(t *testing.B) {
	server := setupBackend(t)
	defer teardownBackend(t, server)
	key := setupKey(t, server, "geheim")
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
		_, err := server.SaveFrom(backend.Data, id[:], uint(size), bytes.NewReader(data))
		OK(t, err)
	}
}

func TestServerStats(t *testing.T) {
	if *benchArchiveDirectory == "" {
		t.Skip("benchdir not set, skipping TestServerStats")
	}

	server := setupBackend(t)
	defer teardownBackend(t, server)
	key := setupKey(t, server, "geheim")
	server.SetKey(key)

	// archive a few files
	sn := snapshot(t, server, *benchArchiveDirectory, nil)
	t.Logf("archived snapshot %v", sn.ID())

	stats, err := server.Stats()
	OK(t, err)
	t.Logf("stats: %v", stats)
}

func TestLoadJSONID(t *testing.T) {
	if *benchArchiveDirectory == "" {
		t.Skip("benchdir not set, skipping TestServerStats")
	}

	server := setupBackend(t)
	defer teardownBackend(t, server)
	key := setupKey(t, server, "geheim")
	server.SetKey(key)

	// archive a few files
	sn := snapshot(t, server, *benchArchiveDirectory, nil)
	t.Logf("archived snapshot %v", sn.ID())

	// benchmark loading first tree
	done := make(chan struct{})
	first, found := <-server.List(backend.Tree, done)
	Assert(t, found, "no Trees in repository found")
	close(done)

	id, err := backend.ParseID(first)
	OK(t, err)

	tree := restic.NewTree()
	err = server.LoadJSONID(backend.Tree, id, &tree)
	OK(t, err)
}

func BenchmarkLoadJSONID(t *testing.B) {
	if *benchArchiveDirectory == "" {
		t.Skip("benchdir not set, skipping TestServerStats")
	}

	server := setupBackend(t)
	defer teardownBackend(t, server)
	key := setupKey(t, server, "geheim")
	server.SetKey(key)

	// archive a few files
	sn := snapshot(t, server, *benchArchiveDirectory, nil)
	t.Logf("archived snapshot %v", sn.ID())

	t.ResetTimer()

	tree := restic.NewTree()
	for i := 0; i < t.N; i++ {
		for name := range server.List(backend.Tree, nil) {
			id, err := backend.ParseID(name)
			OK(t, err)
			OK(t, server.LoadJSONID(backend.Tree, id, &tree))
		}
	}
}
