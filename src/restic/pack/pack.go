package pack

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"sync"

	"restic/backend"
	"restic/crypto"
)

// BlobType specifies what a blob stored in a pack is.
type BlobType uint8

// These are the blob types that can be stored in a pack.
const (
	Invalid BlobType = iota
	Data
	Tree
)

func (t BlobType) String() string {
	switch t {
	case Data:
		return "data"
	case Tree:
		return "tree"
	}

	return fmt.Sprintf("<BlobType %d>", t)
}

// MarshalJSON encodes the BlobType into JSON.
func (t BlobType) MarshalJSON() ([]byte, error) {
	switch t {
	case Data:
		return []byte(`"data"`), nil
	case Tree:
		return []byte(`"tree"`), nil
	}

	return nil, errors.New("unknown blob type")
}

// UnmarshalJSON decodes the BlobType from JSON.
func (t *BlobType) UnmarshalJSON(buf []byte) error {
	switch string(buf) {
	case `"data"`:
		*t = Data
	case `"tree"`:
		*t = Tree
	default:
		return errors.New("unknown blob type")
	}

	return nil
}

// Blob is a blob within a pack.
type Blob struct {
	Type   BlobType
	Length uint
	ID     backend.ID
	Offset uint
}

// GetReader returns an io.Reader for the blob entry e.
func (e Blob) GetReader(rd io.ReadSeeker) (io.Reader, error) {
	// seek to the correct location
	_, err := rd.Seek(int64(e.Offset), 0)
	if err != nil {
		return nil, err
	}

	return io.LimitReader(rd, int64(e.Length)), nil
}

// Packer is used to create a new Pack.
type Packer struct {
	blobs []Blob

	bytes uint
	k     *crypto.Key
	wr    io.Writer

	m sync.Mutex
}

// NewPacker returns a new Packer that can be used to pack blobs
// together. If wr is nil, a bytes.Buffer is used.
func NewPacker(k *crypto.Key, wr io.Writer) *Packer {
	if wr == nil {
		wr = bytes.NewBuffer(nil)
	}
	return &Packer{k: k, wr: wr}
}

// Add saves the data read from rd as a new blob to the packer. Returned is the
// number of bytes written to the pack.
func (p *Packer) Add(t BlobType, id backend.ID, data []byte) (int, error) {
	p.m.Lock()
	defer p.m.Unlock()

	c := Blob{Type: t, ID: id}

	n, err := p.wr.Write(data)
	c.Length = uint(n)
	c.Offset = p.bytes
	p.bytes += uint(n)
	p.blobs = append(p.blobs, c)

	return n, err
}

var entrySize = uint(binary.Size(BlobType(0)) + binary.Size(uint32(0)) + backend.IDSize)

// headerEntry is used with encoding/binary to read and write header entries
type headerEntry struct {
	Type   BlobType
	Length uint32
	ID     [backend.IDSize]byte
}

// Finalize writes the header for all added blobs and finalizes the pack.
// Returned are the number of bytes written, including the header. If the
// underlying writer implements io.Closer, it is closed.
func (p *Packer) Finalize() (uint, error) {
	p.m.Lock()
	defer p.m.Unlock()

	bytesWritten := p.bytes

	hdrBuf := bytes.NewBuffer(nil)
	bytesHeader, err := p.writeHeader(hdrBuf)
	if err != nil {
		return 0, err
	}

	encryptedHeader, err := crypto.Encrypt(p.k, nil, hdrBuf.Bytes())
	if err != nil {
		return 0, err
	}

	// append the header
	n, err := p.wr.Write(encryptedHeader)
	if err != nil {
		return 0, err
	}

	hdrBytes := bytesHeader + crypto.Extension
	if uint(n) != hdrBytes {
		return 0, errors.New("wrong number of bytes written")
	}

	bytesWritten += hdrBytes

	// write length
	err = binary.Write(p.wr, binary.LittleEndian, uint32(uint(len(p.blobs))*entrySize+crypto.Extension))
	if err != nil {
		return 0, err
	}
	bytesWritten += uint(binary.Size(uint32(0)))

	p.bytes = uint(bytesWritten)

	if w, ok := p.wr.(io.Closer); ok {
		return bytesWritten, w.Close()
	}

	return bytesWritten, nil
}

// writeHeader constructs and writes the header to wr.
func (p *Packer) writeHeader(wr io.Writer) (bytesWritten uint, err error) {
	for _, b := range p.blobs {
		entry := headerEntry{
			Type:   b.Type,
			Length: uint32(b.Length),
			ID:     b.ID,
		}

		err := binary.Write(wr, binary.LittleEndian, entry)
		if err != nil {
			return bytesWritten, err
		}

		bytesWritten += entrySize
	}

	return
}

// Size returns the number of bytes written so far.
func (p *Packer) Size() uint {
	p.m.Lock()
	defer p.m.Unlock()

	return p.bytes
}

// Count returns the number of blobs in this packer.
func (p *Packer) Count() int {
	p.m.Lock()
	defer p.m.Unlock()

	return len(p.blobs)
}

// Blobs returns the slice of blobs that have been written.
func (p *Packer) Blobs() []Blob {
	p.m.Lock()
	defer p.m.Unlock()

	return p.blobs
}

// Writer return the underlying writer.
func (p *Packer) Writer() io.Writer {
	return p.wr
}

func (p *Packer) String() string {
	return fmt.Sprintf("<Packer %d blobs, %d bytes>", len(p.blobs), p.bytes)
}

// Unpacker is used to read individual blobs from a pack.
type Unpacker struct {
	rd      io.ReadSeeker
	Entries []Blob
	k       *crypto.Key
}

// NewUnpacker returns a pointer to Unpacker which can be used to read
// individual Blobs from a pack.
func NewUnpacker(k *crypto.Key, rd io.ReadSeeker) (*Unpacker, error) {
	var err error
	ls := binary.Size(uint32(0))

	// reset to the end to read header length
	_, err = rd.Seek(-int64(ls), 2)
	if err != nil {
		return nil, fmt.Errorf("seeking to read header length failed: %v", err)
	}

	var length uint32
	err = binary.Read(rd, binary.LittleEndian, &length)
	if err != nil {
		return nil, fmt.Errorf("reading header length failed: %v", err)
	}

	// reset to the beginning of the header
	_, err = rd.Seek(-int64(ls)-int64(length), 2)
	if err != nil {
		return nil, fmt.Errorf("seeking to read header length failed: %v", err)
	}

	// read header
	hrd, err := crypto.DecryptFrom(k, io.LimitReader(rd, int64(length)))
	if err != nil {
		return nil, err
	}

	var entries []Blob

	pos := uint(0)
	for {
		e := headerEntry{}
		err = binary.Read(hrd, binary.LittleEndian, &e)
		if err == io.EOF {
			break
		}

		if err != nil {
			return nil, err
		}

		entries = append(entries, Blob{
			Type:   e.Type,
			Length: uint(e.Length),
			ID:     e.ID,
			Offset: pos,
		})

		pos += uint(e.Length)
	}

	p := &Unpacker{
		rd:      rd,
		k:       k,
		Entries: entries,
	}

	return p, nil
}
