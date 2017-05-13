package test

import (
	"bytes"
	"io"
	"restic"
	"restic/test"
	"testing"
)

// BackendBenchmarkLoad benchmarks the backend's Load function.
func BackendBenchmarkLoadFile(t *testing.B, s *Suite) {
	be := s.open(t)
	defer s.close(t, be)

	length := 1<<24 + 2123
	data := test.Random(23, length)
	id := restic.Hash(data)
	handle := restic.Handle{Type: restic.DataFile, Name: id.String()}
	if err := be.Save(handle, bytes.NewReader(data)); err != nil {
		t.Fatalf("Save() error: %+v", err)
	}

	defer func() {
		if err := be.Remove(handle); err != nil {
			t.Fatalf("Remove() returned error: %v", err)
		}
	}()

	buf := make([]byte, length)

	t.SetBytes(int64(length))
	t.ResetTimer()

	for i := 0; i < t.N; i++ {
		rd, err := be.Load(handle, 0, 0)
		if err != nil {
			t.Fatal(err)
		}

		n, err := io.ReadFull(rd, buf)
		if err != nil {
			t.Fatal(err)
		}

		if err = rd.Close(); err != nil {
			t.Fatalf("Close() returned error: %v", err)
		}

		if n != length {
			t.Fatalf("wrong number of bytes read: want %v, got %v", length, n)
		}

		if !bytes.Equal(data, buf) {
			t.Fatalf("wrong bytes returned")
		}

	}
}
