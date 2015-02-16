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
	be := setupBackend(t)
	defer teardownBackend(t, be)
	key := setupKey(t, be, "geheim")
	server := restic.NewServerWithKey(be, key)

	for _, obj := range serverTests {
		data, err := json.Marshal(obj)
		ok(t, err)
		data = append(data, '\n')
		h := sha256.Sum256(data)

		blob, err := server.SaveJSON(backend.Tree, obj)
		ok(t, err)

		assert(t, bytes.Equal(h[:], blob.ID),
			"TestSaveJSON: wrong plaintext ID: expected %02x, got %02x",
			h, blob.ID)
	}
}

func BenchmarkSaveJSON(t *testing.B) {
	be := setupBackend(t)
	defer teardownBackend(t, be)
	key := setupKey(t, be, "geheim")
	server := restic.NewServerWithKey(be, key)

	obj := serverTests[0]

	data, err := json.Marshal(obj)
	ok(t, err)
	data = append(data, '\n')
	h := sha256.Sum256(data)

	t.ResetTimer()

	for i := 0; i < t.N; i++ {
		blob, err := server.SaveJSON(backend.Tree, obj)
		ok(t, err)

		assert(t, bytes.Equal(h[:], blob.ID),
			"TestSaveJSON: wrong plaintext ID: expected %02x, got %02x",
			h, blob.ID)
	}
}

var testSizes = []int{5, 23, 2<<18 + 23, 1 << 20}

func TestSaveFrom(t *testing.T) {
	be := setupBackend(t)
	defer teardownBackend(t, be)
	key := setupKey(t, be, "geheim")
	server := restic.NewServerWithKey(be, key)

	for _, size := range testSizes {
		data := make([]byte, size)
		_, err := io.ReadFull(rand.Reader, data)
		ok(t, err)

		id := sha256.Sum256(data)

		// save
		blob, err := server.SaveFrom(backend.Data, id[:], uint(size), bytes.NewReader(data))
		ok(t, err)

		// read back
		buf, err := server.Load(backend.Data, blob)

		assert(t, len(buf) == len(data),
			"number of bytes read back does not match: expected %d, got %d",
			len(data), len(buf))

		assert(t, bytes.Equal(buf, data),
			"data does not match: expected %02x, got %02x",
			data, buf)
	}
}

func BenchmarkSaveFrom(t *testing.B) {
	be := setupBackend(t)
	defer teardownBackend(t, be)
	key := setupKey(t, be, "geheim")
	server := restic.NewServerWithKey(be, key)

	size := 4 << 20 // 4MiB

	data := make([]byte, size)
	_, err := io.ReadFull(rand.Reader, data)
	ok(t, err)

	id := sha256.Sum256(data)

	t.ResetTimer()
	t.SetBytes(int64(size))

	for i := 0; i < t.N; i++ {
		// save
		_, err := server.SaveFrom(backend.Data, id[:], uint(size), bytes.NewReader(data))
		ok(t, err)
	}
}
