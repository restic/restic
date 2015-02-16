package restic_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
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

	t.ResetTimer()
	//t.SetBytes(int64(size))

	obj := serverTests[0]

	data, err := json.Marshal(obj)
	ok(t, err)
	data = append(data, '\n')
	h := sha256.Sum256(data)

	for i := 0; i < t.N; i++ {
		blob, err := server.SaveJSON(backend.Tree, obj)
		ok(t, err)

		assert(t, bytes.Equal(h[:], blob.ID),
			"TestSaveJSON: wrong plaintext ID: expected %02x, got %02x",
			h, blob.ID)
	}
}
