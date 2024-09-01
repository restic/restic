package hashing

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"io"
	"testing"
)

func TestReader(t *testing.T) {
	tests := []int{5, 23, 2<<18 + 23, 1 << 20}

	for _, size := range tests {
		data := make([]byte, size)
		_, err := io.ReadFull(rand.Reader, data)
		if err != nil {
			t.Fatalf("ReadFull: %v", err)
		}

		expectedHash := sha256.Sum256(data)

		rd := NewReader(bytes.NewReader(data), sha256.New())
		n, err := io.Copy(io.Discard, rd)
		if err != nil {
			t.Fatal(err)
		}

		if n != int64(size) {
			t.Errorf("Reader: invalid number of bytes written: got %d, expected %d",
				n, size)
		}

		resultingHash := rd.Sum(nil)

		if !bytes.Equal(expectedHash[:], resultingHash) {
			t.Errorf("Reader: hashes do not match: expected %02x, got %02x",
				expectedHash, resultingHash)
		}
	}
}

func BenchmarkReader(b *testing.B) {
	buf := make([]byte, 1<<22)
	_, err := io.ReadFull(rand.Reader, buf)
	if err != nil {
		b.Fatal(err)
	}

	expectedHash := sha256.Sum256(buf)

	b.SetBytes(int64(len(buf)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rd := NewReader(bytes.NewReader(buf), sha256.New())
		n, err := io.Copy(io.Discard, rd)
		if err != nil {
			b.Fatal(err)
		}

		if n != int64(len(buf)) {
			b.Errorf("Reader: invalid number of bytes written: got %d, expected %d",
				n, len(buf))
		}

		resultingHash := rd.Sum(nil)
		if !bytes.Equal(expectedHash[:], resultingHash) {
			b.Errorf("Reader: hashes do not match: expected %02x, got %02x",
				expectedHash, resultingHash)
		}
	}
}
