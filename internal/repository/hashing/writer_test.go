package hashing

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"io"
	"testing"
)

func TestWriter(t *testing.T) {
	tests := []int{5, 23, 2<<18 + 23, 1 << 20}

	for _, size := range tests {
		data := make([]byte, size)
		_, err := io.ReadFull(rand.Reader, data)
		if err != nil {
			t.Fatalf("ReadFull: %v", err)
		}

		expectedHash := sha256.Sum256(data)

		wr := NewWriter(io.Discard, sha256.New())

		n, err := io.Copy(wr, bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}

		if n != int64(size) {
			t.Errorf("Writer: invalid number of bytes written: got %d, expected %d",
				n, size)
		}

		resultingHash := wr.Sum(nil)

		if !bytes.Equal(expectedHash[:], resultingHash) {
			t.Errorf("Writer: hashes do not match: expected %02x, got %02x",
				expectedHash, resultingHash)
		}
	}
}

func BenchmarkWriter(b *testing.B) {
	buf := make([]byte, 1<<22)
	_, err := io.ReadFull(rand.Reader, buf)
	if err != nil {
		b.Fatal(err)
	}

	expectedHash := sha256.Sum256(buf)

	b.SetBytes(int64(len(buf)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		wr := NewWriter(io.Discard, sha256.New())
		n, err := io.Copy(wr, bytes.NewReader(buf))
		if err != nil {
			b.Fatal(err)
		}

		if n != int64(len(buf)) {
			b.Errorf("Writer: invalid number of bytes written: got %d, expected %d",
				n, len(buf))
		}

		resultingHash := wr.Sum(nil)
		if !bytes.Equal(expectedHash[:], resultingHash) {
			b.Errorf("Writer: hashes do not match: expected %02x, got %02x",
				expectedHash, resultingHash)
		}
	}
}
