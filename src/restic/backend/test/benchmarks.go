package test

import (
	"bytes"
	"io"
	"restic"
	"restic/test"
	"testing"
)

func saveRandomFile(t testing.TB, be restic.Backend, length int) ([]byte, restic.Handle) {
	data := test.Random(23, length)
	id := restic.Hash(data)
	handle := restic.Handle{Type: restic.DataFile, Name: id.String()}
	if err := be.Save(handle, bytes.NewReader(data)); err != nil {
		t.Fatalf("Save() error: %+v", err)
	}
	return data, handle
}

func remove(t testing.TB, be restic.Backend, h restic.Handle) {
	if err := be.Remove(h); err != nil {
		t.Fatalf("Remove() returned error: %v", err)
	}
}

func BackendBenchmarkLoadFile(t *testing.B, s *Suite) {
	be := s.open(t)
	defer s.close(t, be)

	length := 1<<24 + 2123
	data, handle := saveRandomFile(t, be, length)
	defer remove(t, be, handle)

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

func BackendBenchmarkLoadPartialFile(t *testing.B, s *Suite) {
	be := s.open(t)
	defer s.close(t, be)

	datalength := 1<<24 + 2123
	data, handle := saveRandomFile(t, be, datalength)
	defer remove(t, be, handle)

	testLength := datalength/4 + 555

	buf := make([]byte, testLength)

	t.SetBytes(int64(testLength))
	t.ResetTimer()

	for i := 0; i < t.N; i++ {
		rd, err := be.Load(handle, testLength, 0)
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

		if n != testLength {
			t.Fatalf("wrong number of bytes read: want %v, got %v", testLength, n)
		}

		if !bytes.Equal(data[:testLength], buf) {
			t.Fatalf("wrong bytes returned")
		}

	}
}

func BackendBenchmarkLoadPartialFileOffset(t *testing.B, s *Suite) {
	be := s.open(t)
	defer s.close(t, be)

	datalength := 1<<24 + 2123
	data, handle := saveRandomFile(t, be, datalength)
	defer remove(t, be, handle)

	testLength := datalength/4 + 555
	testOffset := 8273

	buf := make([]byte, testLength)

	t.SetBytes(int64(testLength))
	t.ResetTimer()

	for i := 0; i < t.N; i++ {
		rd, err := be.Load(handle, testLength, int64(testOffset))
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

		if n != testLength {
			t.Fatalf("wrong number of bytes read: want %v, got %v", testLength, n)
		}

		if !bytes.Equal(data[testOffset:testOffset+testLength], buf) {
			t.Fatalf("wrong bytes returned")
		}

	}
}
