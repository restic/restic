package backend

import (
	"bytes"
	"hash"
	"io"

	"github.com/restic/restic/internal/errors"
)

// RewindReader allows resetting the Reader to the beginning of the data.
type RewindReader interface {
	io.Reader

	// Rewind rewinds the reader so the same data can be read again from the
	// start.
	Rewind() error

	// Length returns the number of bytes that can be read from the Reader
	// after calling Rewind.
	Length() int64

	// Hash return a hash of the data if requested by the backed.
	Hash() []byte
}

// ByteReader implements a RewindReader for a byte slice.
type ByteReader struct {
	*bytes.Reader
	Len  int64
	hash []byte
}

// Rewind restarts the reader from the beginning of the data.
func (b *ByteReader) Rewind() error {
	_, err := b.Reader.Seek(0, io.SeekStart)
	return err
}

// Length returns the number of bytes read from the reader after Rewind is
// called.
func (b *ByteReader) Length() int64 {
	return b.Len
}

// Hash return a hash of the data if requested by the backed.
func (b *ByteReader) Hash() []byte {
	return b.hash
}

// statically ensure that *ByteReader implements RewindReader.
var _ RewindReader = &ByteReader{}

// NewByteReader prepares a ByteReader that can then be used to read buf.
func NewByteReader(buf []byte, hasher hash.Hash) *ByteReader {
	var hash []byte
	if hasher != nil {
		// must never fail according to interface
		_, err := hasher.Write(buf)
		if err != nil {
			panic(err)
		}
		hash = hasher.Sum(nil)
	}
	return &ByteReader{
		Reader: bytes.NewReader(buf),
		Len:    int64(len(buf)),
		hash:   hash,
	}
}

// statically ensure that *FileReader implements RewindReader.
var _ RewindReader = &FileReader{}

// FileReader implements a RewindReader for an open file.
type FileReader struct {
	io.ReadSeeker
	Len  int64
	hash []byte
}

// Rewind seeks to the beginning of the file.
func (f *FileReader) Rewind() error {
	_, err := f.ReadSeeker.Seek(0, io.SeekStart)
	return errors.Wrap(err, "Seek")
}

// Length returns the length of the file.
func (f *FileReader) Length() int64 {
	return f.Len
}

// Hash return a hash of the data if requested by the backed.
func (f *FileReader) Hash() []byte {
	return f.hash
}

// NewFileReader wraps f in a *FileReader.
func NewFileReader(f io.ReadSeeker, hash []byte) (*FileReader, error) {
	pos, err := f.Seek(0, io.SeekEnd)
	if err != nil {
		return nil, errors.Wrap(err, "Seek")
	}

	fr := &FileReader{
		ReadSeeker: f,
		Len:        pos,
		hash:       hash,
	}

	err = fr.Rewind()
	if err != nil {
		return nil, err
	}

	return fr, nil
}
