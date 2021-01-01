package hashing

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"io"
	"io/ioutil"
	"testing"

	rtest "github.com/restic/restic/internal/test"
)

// only expose Read method
type onlyReader struct {
	io.Reader
}

type traceWriterTo struct {
	io.Reader
	writerTo io.WriterTo
	Traced   bool
}

func (r *traceWriterTo) WriteTo(w io.Writer) (n int64, err error) {
	r.Traced = true
	return r.writerTo.WriteTo(w)
}

func TestReader(t *testing.T) {
	tests := []int{5, 23, 2<<18 + 23, 1 << 20}

	for _, size := range tests {
		data := make([]byte, size)
		_, err := io.ReadFull(rand.Reader, data)
		if err != nil {
			t.Fatalf("ReadFull: %v", err)
		}

		expectedHash := sha256.Sum256(data)

		for _, test := range []struct {
			innerWriteTo, outerWriteTo bool
		}{{false, false}, {false, true}, {true, false}, {true, true}} {
			// test both code paths in WriteTo
			src := bytes.NewReader(data)
			rawSrc := &traceWriterTo{Reader: src, writerTo: src}
			innerSrc := io.Reader(rawSrc)
			if !test.innerWriteTo {
				innerSrc = &onlyReader{Reader: rawSrc}
			}

			rd := NewReader(innerSrc, sha256.New())
			// test both Read and WriteTo
			outerSrc := io.Reader(rd)
			if !test.outerWriteTo {
				outerSrc = &onlyReader{Reader: outerSrc}
			}

			n, err := io.Copy(ioutil.Discard, outerSrc)
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

			rtest.Assert(t, rawSrc.Traced == (test.innerWriteTo && test.outerWriteTo),
				"unexpected/missing writeTo call innerWriteTo %v outerWriteTo %v",
				test.innerWriteTo, test.outerWriteTo)
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
		n, err := io.Copy(ioutil.Discard, rd)
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
