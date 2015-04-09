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

func TestHashAppendWriter(t *testing.T) {
	tests := []int{5, 23, 2<<18 + 23, 1 << 20}

	for _, size := range tests {
		data := make([]byte, size)
		_, err := io.ReadFull(rand.Reader, data)
		if err != nil {
			t.Fatalf("ReadFull: %v", err)
		}

		expectedHash := sha256.Sum256(data)

		target := bytes.NewBuffer(nil)
		wr := backend.NewHashAppendWriter(target, sha256.New())

		_, err = wr.Write(data)
		OK(t, err)
		OK(t, wr.Close())

		Assert(t, len(target.Bytes()) == size+len(expectedHash),
			"HashAppendWriter: invalid number of bytes written: got %d, expected %d",
			len(target.Bytes()), size+len(expectedHash))

		r := target.Bytes()
		resultingHash := r[len(r)-len(expectedHash):]
		Assert(t, bytes.Equal(expectedHash[:], resultingHash),
			"HashAppendWriter: hashes do not match: expected %02x, got %02x",
			expectedHash, resultingHash)

		// write again, this must return an error
		_, err = wr.Write([]byte{23})
		Assert(t, err != nil,
			"HashAppendWriter: Write() after Close() did not return an error")
	}
}

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
