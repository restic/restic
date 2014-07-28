package khepri

import (
	"hash"
	"io"
)

// HashingReader is the interfaces that wraps a normal reader. When Hash() is called,
// it returns the hash for all data that has been read so far.
type HashingReader interface {
	io.Reader
	Hash() []byte
}

// HashingWriter is the interfaces that wraps a normal writer. When Hash() is called,
// it returns the hash for all data that has been written so far.
type HashingWriter interface {
	io.Writer
	Hash() []byte
}

type reader struct {
	reader io.Reader
	hash   hash.Hash
}

// NewHashingReader wraps an io.Reader and in addition feeds all data read through the
// given hash.
func NewHashingReader(r io.Reader, h func() hash.Hash) *reader {
	return &reader{
		reader: r,
		hash:   h(),
	}
}

func (h *reader) Read(p []byte) (int, error) {
	// call original reader
	n, err := h.reader.Read(p)

	// hash bytes
	if n > 0 {
		// hash
		h.hash.Write(p[0:n])
	}

	// return result
	return n, err
}

func (h *reader) Hash() []byte {
	return h.hash.Sum([]byte{})
}

type writer struct {
	writer io.Writer
	hash   hash.Hash
}

// NewHashingWriter wraps an io.Reader and in addition feeds all data written through
// the given hash.
func NewHashingWriter(w io.Writer, h func() hash.Hash) *writer {
	return &writer{
		writer: w,
		hash:   h(),
	}
}

func (h *writer) Write(p []byte) (int, error) {
	// call original writer
	n, err := h.writer.Write(p)

	// hash bytes
	if n > 0 {
		// hash
		h.hash.Write(p[0:n])
	}

	// return result
	return n, err
}

func (h *writer) Hash() []byte {
	return h.hash.Sum([]byte{})
}
