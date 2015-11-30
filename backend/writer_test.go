package backend_test

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"io"
	"io/ioutil"
	"testing"

	"github.com/restic/restic/backend"
	. "github.com/restic/restic/test"
)

func TestHashingWriter(t *testing.T) {
	tests := []int{5, 23, 2<<18 + 23, 1 << 20}

	for _, size := range tests {
		data := make([]byte, size)
		_, err := io.ReadFull(rand.Reader, data)
		if err != nil {
			t.Fatalf("ReadFull: %v", err)
		}

		expectedHash := sha256.Sum256(data)

		wr := backend.NewHashingWriter(ioutil.Discard, sha256.New())

		n, err := io.Copy(wr, bytes.NewReader(data))
		OK(t, err)

		Assert(t, n == int64(size),
			"HashAppendWriter: invalid number of bytes written: got %d, expected %d",
			n, size)

		Assert(t, wr.Size() == size,
			"HashAppendWriter: invalid number of bytes returned: got %d, expected %d",
			wr.Size, size)

		resultingHash := wr.Sum(nil)
		Assert(t, bytes.Equal(expectedHash[:], resultingHash),
			"HashAppendWriter: hashes do not match: expected %02x, got %02x",
			expectedHash, resultingHash)
	}
}
