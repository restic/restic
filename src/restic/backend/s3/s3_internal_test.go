package s3

import (
	"bytes"
	"io"
	"io/ioutil"
	"os"
	"restic/test"
	"testing"
)

func writeFile(t testing.TB, data []byte, offset int64) *os.File {
	tempfile, err := ioutil.TempFile("", "restic-test-")
	if err != nil {
		t.Fatal(err)
	}

	if _, err = tempfile.Write(data); err != nil {
		t.Fatal(err)
	}

	if _, err = tempfile.Seek(offset, io.SeekStart); err != nil {
		t.Fatal(err)
	}

	return tempfile
}

func TestGetRemainingSize(t *testing.T) {
	length := 18 * 1123
	partialRead := 1005

	data := test.Random(23, length)

	partReader := bytes.NewReader(data)
	buf := make([]byte, partialRead)
	_, _ = io.ReadFull(partReader, buf)

	partFileReader := writeFile(t, data, int64(partialRead))
	defer func() {
		if err := partFileReader.Close(); err != nil {
			t.Fatal(err)
		}

		if err := os.Remove(partFileReader.Name()); err != nil {
			t.Fatal(err)
		}
	}()

	var tests = []struct {
		io.Reader
		size int64
	}{
		{bytes.NewReader([]byte("foobar test")), 11},
		{partReader, int64(length - partialRead)},
		{partFileReader, int64(length - partialRead)},
	}

	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			size, err := getRemainingSize(test.Reader)
			if err != nil {
				t.Fatal(err)
			}

			if size != test.size {
				t.Fatalf("invalid size returned, want %v, got %v", test.size, size)
			}
		})
	}
}
