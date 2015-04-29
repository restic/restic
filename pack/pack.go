package pack

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/restic/restic/backend"
	"github.com/restic/restic/crypto"
)

type BlobType uint8

const (
	Data BlobType = 0
	Tree          = 1
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

func (t BlobType) MarshalJSON() ([]byte, error) {
	switch t {
	case Data:
		return []byte(`"data"`), nil
	case Tree:
		return []byte(`"tree"`), nil
	}

	return nil, errors.New("unknown blob type")
}

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
	Length uint32
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
	hw    *backend.HashingWriter

	m sync.Mutex
}

// NewPacker returns a new Packer that can be used to pack blobs
// together.
func NewPacker(k *crypto.Key, w io.Writer) *Packer {
	return &Packer{k: k, wr: w, hw: backend.NewHashingWriter(w, sha256.New())}
}

// Add saves the data read from rd as a new blob to the packer. Returned is the
// number of bytes written to the pack.
func (p *Packer) Add(t BlobType, id backend.ID, rd io.Reader) (int64, error) {
	p.m.Lock()
	defer p.m.Unlock()

	c := Blob{Type: t, ID: id}

	n, err := io.Copy(p.hw, rd)
	c.Length = uint32(n)
	c.Offset = p.bytes
	p.bytes += uint(n)
	p.blobs = append(p.blobs, c)

	return n, err
}

var entrySize = binary.Size(BlobType(0)) + binary.Size(uint32(0)) + backend.IDSize

// headerEntry is used with encoding/binary to read and write header entries
type headerEntry struct {
	Type   BlobType
	Length uint32
	ID     [backend.IDSize]byte
}

// Finalize writes the header for all added blobs and finalizes the pack.
// Returned are the complete number of bytes written, including the header.
// After Finalize() has finished, the ID of this pack can be obtained by
// calling ID().
func (p *Packer) Finalize() (bytesWritten int64, err error) {
	p.m.Lock()
	defer p.m.Unlock()

	bytesWritten = int64(p.bytes)

	// create writer to encrypt header
	wr := crypto.EncryptTo(p.k, p.hw)

	// write header
	for _, b := range p.blobs {
		entry := headerEntry{
			Type:   b.Type,
			Length: b.Length,
		}
		copy(entry.ID[:], b.ID)

		err := binary.Write(wr, binary.LittleEndian, entry)
		if err != nil {
			wr.Close()
			return int64(bytesWritten), err
		}

		bytesWritten += int64(entrySize)
	}

	// finalize encrypted header
	err = wr.Close()
	if err != nil {
		return int64(bytesWritten), err
	}

	// account for crypto overhead
	bytesWritten += crypto.Extension

	// write length
	err = binary.Write(p.hw, binary.LittleEndian, uint32(len(p.blobs)*entrySize+crypto.Extension))
	if err != nil {
		return bytesWritten, err
	}
	bytesWritten += int64(binary.Size(uint32(0)))

	p.bytes = uint(bytesWritten)

	return bytesWritten, nil
}

// ID returns the ID of all data written so far.
func (p *Packer) ID() backend.ID {
	p.m.Lock()
	defer p.m.Unlock()

	return p.hw.Sum(nil)
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

// Writer returns the underlying writer.
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
func NewUnpacker(k *crypto.Key, entries []Blob, rd io.ReadSeeker) (*Unpacker, error) {
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

	if entries == nil {
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
				Length: e.Length,
				ID:     e.ID[:],
				Offset: pos,
			})

			pos += uint(e.Length)
		}
	}

	p := &Unpacker{
		rd:      rd,
		k:       k,
		Entries: entries,
	}

	return p, nil
}
