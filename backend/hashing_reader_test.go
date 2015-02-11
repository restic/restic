package backend_test

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"io"
	"testing"

	"github.com/restic/restic/backend"
)

func TestHashReader(t *testing.T) {
	tests := []int{5, 23, 2<<18 + 23, 1 << 20}

	for _, size := range tests {
		data := make([]byte, size)
		_, err := io.ReadFull(rand.Reader, data)
		if err != nil {
			t.Fatalf("ReadFull: %v", err)
		}

		expectedHash := sha256.Sum256(data)

		rd := backend.NewHashReader(bytes.NewReader(data), sha256.New())

		target := bytes.NewBuffer(nil)
		n, err := io.Copy(target, rd)
		ok(t, err)

		assert(t, n == int64(size)+int64(len(expectedHash)),
			"HashReader: invalid number of bytes read: got %d, expected %d",
			n, size+len(expectedHash))

		r := target.Bytes()
		resultingHash := r[len(r)-len(expectedHash):]
		assert(t, bytes.Equal(expectedHash[:], resultingHash),
			"HashReader: hashes do not match: expected %02x, got %02x",
			expectedHash, resultingHash)

		// try to read again, must return io.EOF
		n2, err := rd.Read(make([]byte, 100))
		assert(t, n2 == 0, "HashReader returned %d additional bytes", n)
		assert(t, err == io.EOF, "HashReader returned %v instead of EOF", err)
	}
}
