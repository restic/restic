package backend_test

import (
	"bytes"
	"io"
	"math/rand"
	"restic/backend"
	"restic/backend/mem"
	"testing"

	. "restic/test"
)

func abs(a int) int {
	if a < 0 {
		return -a
	}

	return a
}

func loadAndCompare(t testing.TB, rd io.ReadSeeker, size int, offset int64, expected []byte) {
	var (
		pos int64
		err error
	)

	if offset >= 0 {
		pos, err = rd.Seek(offset, 0)
	} else {
		pos, err = rd.Seek(offset, 2)
	}
	if err != nil {
		t.Errorf("Seek(%d, 0) returned error: %v", offset, err)
		return
	}

	if offset >= 0 && pos != offset {
		t.Errorf("pos after seek is wrong, want %d, got %d", offset, pos)
	} else if offset < 0 && pos != int64(size)+offset {
		t.Errorf("pos after relative seek is wrong, want %d, got %d", int64(size)+offset, pos)
	}

	buf := make([]byte, len(expected))
	n, err := rd.Read(buf)

	// if we requested data beyond the end of the file, ignore
	// ErrUnexpectedEOF error
	if offset > 0 && len(buf) > size && err == io.ErrUnexpectedEOF {
		err = nil
		buf = buf[:size]
	}

	if offset < 0 && len(buf) > abs(int(offset)) && err == io.ErrUnexpectedEOF {
		err = nil
		buf = buf[:abs(int(offset))]
	}

	if n != len(buf) {
		t.Errorf("Load(%d, %d): wrong length returned, want %d, got %d",
			len(buf), offset, len(buf), n)
		return
	}

	if err != nil {
		t.Errorf("Load(%d, %d): unexpected error: %v", len(buf), offset, err)
		return
	}

	buf = buf[:n]
	if !bytes.Equal(buf, expected) {
		t.Errorf("Load(%d, %d) returned wrong bytes", len(buf), offset)
		return
	}
}

func TestReadSeeker(t *testing.T) {
	b := mem.New()

	length := rand.Intn(1<<24) + 2000

	data := Random(23, length)
	id := backend.Hash(data)

	handle := backend.Handle{Type: backend.Data, Name: id.String()}
	err := b.Save(handle, data)
	if err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	for i := 0; i < 50; i++ {
		l := rand.Intn(length + 2000)
		o := rand.Intn(length + 2000)

		if rand.Float32() > 0.5 {
			o = -o
		}

		d := data
		if o > 0 && o < len(d) {
			d = d[o:]
		} else {
			o = len(d)
			d = d[:0]
		}

		if l > 0 && l < len(d) {
			d = d[:l]
		}

		rd := backend.NewReadSeeker(b, handle)
		loadAndCompare(t, rd, len(data), int64(o), d)
	}
}
