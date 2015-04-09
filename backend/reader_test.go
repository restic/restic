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

func TestHashAppendReader(t *testing.T) {
	tests := []int{5, 23, 2<<18 + 23, 1 << 20}

	for _, size := range tests {
		data := make([]byte, size)
		_, err := io.ReadFull(rand.Reader, data)
		if err != nil {
			t.Fatalf("ReadFull: %v", err)
		}

		expectedHash := sha256.Sum256(data)

		rd := backend.NewHashAppendReader(bytes.NewReader(data), sha256.New())

		target := bytes.NewBuffer(nil)
		n, err := io.Copy(target, rd)
		OK(t, err)

		Assert(t, n == int64(size)+int64(len(expectedHash)),
			"HashAppendReader: invalid number of bytes read: got %d, expected %d",
			n, size+len(expectedHash))

		r := target.Bytes()
		resultingHash := r[len(r)-len(expectedHash):]
		Assert(t, bytes.Equal(expectedHash[:], resultingHash),
			"HashAppendReader: hashes do not match: expected %02x, got %02x",
			expectedHash, resultingHash)

		// try to read again, must return io.EOF
		n2, err := rd.Read(make([]byte, 100))
		Assert(t, n2 == 0, "HashAppendReader returned %d additional bytes", n)
		Assert(t, err == io.EOF, "HashAppendReader returned %v instead of EOF", err)
	}
}

func TestHashingReader(t *testing.T) {
	tests := []int{5, 23, 2<<18 + 23, 1 << 20}

	for _, size := range tests {
		data := make([]byte, size)
		_, err := io.ReadFull(rand.Reader, data)
		if err != nil {
			t.Fatalf("ReadFull: %v", err)
		}

		expectedHash := sha256.Sum256(data)

		rd := backend.NewHashingReader(bytes.NewReader(data), sha256.New())

		n, err := io.Copy(ioutil.Discard, rd)
		OK(t, err)

		Assert(t, n == int64(size),
			"HashAppendReader: invalid number of bytes read: got %d, expected %d",
			n, size)

		resultingHash := rd.Sum(nil)
		Assert(t, bytes.Equal(expectedHash[:], resultingHash),
			"HashAppendReader: hashes do not match: expected %02x, got %02x",
			expectedHash, resultingHash)

		// try to read again, must return io.EOF
		n2, err := rd.Read(make([]byte, 100))
		Assert(t, n2 == 0, "HashAppendReader returned %d additional bytes", n)
		Assert(t, err == io.EOF, "HashAppendReader returned %v instead of EOF", err)
	}
}
