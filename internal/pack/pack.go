package pack

import (
	"encoding/binary"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/restic"

	"github.com/restic/restic/internal/crypto"
)

// Packer is used to create a new Pack.
type Packer struct {
	blobs []restic.Blob

	bytes   uint
	created time.Time
	k       *crypto.Key
	wr      io.Writer

	m sync.Mutex
}

// NewPacker returns a new Packer that can be used to pack blobs together.
func NewPacker(k *crypto.Key, wr io.Writer) *Packer {
	return &Packer{k: k, wr: wr, created: time.Now()}
}

// Add saves the data read from rd as a new blob to the packer. Returned is the
// number of bytes written to the pack.
func (p *Packer) Add(t restic.BlobType, id restic.ID, data []byte) (int, error) {
	p.m.Lock()
	defer p.m.Unlock()

	c := restic.Blob{BlobHandle: restic.BlobHandle{Type: t, ID: id}}

	n, err := p.wr.Write(data)
	c.Length = uint(n)
	c.Offset = p.bytes
	p.bytes += uint(n)
	p.blobs = append(p.blobs, c)

	return n, errors.Wrap(err, "Write")
}

var EntrySize = uint(binary.Size(restic.BlobType(0)) + headerLengthSize + len(restic.ID{}))

// headerEntry describes the format of header entries. It serves only as
// documentation.
type headerEntry struct {
	Type   uint8
	Length uint32
	ID     restic.ID
}

// Finalize writes the header for all added blobs and finalizes the pack.
// Returned are the number of bytes written, including the header.
func (p *Packer) Finalize() (uint, error) {
	p.m.Lock()
	defer p.m.Unlock()

	bytesWritten := p.bytes

	header, err := p.makeHeader()
	if err != nil {
		return 0, err
	}

	encryptedHeader := make([]byte, 0, len(header)+p.k.Overhead()+p.k.NonceSize())
	nonce := crypto.NewRandomNonce()
	encryptedHeader = append(encryptedHeader, nonce...)
	encryptedHeader = p.k.Seal(encryptedHeader, nonce, header, nil)

	// append the header
	n, err := p.wr.Write(encryptedHeader)
	if err != nil {
		return 0, errors.Wrap(err, "Write")
	}

	hdrBytes := restic.CiphertextLength(len(header))
	if n != hdrBytes {
		return 0, errors.New("wrong number of bytes written")
	}

	bytesWritten += uint(hdrBytes)

	// write length
	err = binary.Write(p.wr, binary.LittleEndian, uint32(restic.CiphertextLength(len(p.blobs)*int(EntrySize))))
	if err != nil {
		return 0, errors.Wrap(err, "binary.Write")
	}
	bytesWritten += uint(binary.Size(uint32(0)))

	p.bytes = uint(bytesWritten)
	return bytesWritten, nil
}

// makeHeader constructs the header for p.
func (p *Packer) makeHeader() ([]byte, error) {
	buf := make([]byte, 0, len(p.blobs)*int(EntrySize))

	for _, b := range p.blobs {
		switch b.Type {
		case restic.DataBlob:
			buf = append(buf, 0)
		case restic.TreeBlob:
			buf = append(buf, 1)
		default:
			return nil, errors.Errorf("invalid blob type %v", b.Type)
		}

		var lenLE [4]byte
		binary.LittleEndian.PutUint32(lenLE[:], uint32(b.Length))
		buf = append(buf, lenLE[:]...)
		buf = append(buf, b.ID[:]...)
	}

	return buf, nil
}

const packMaxAge = 5 * time.Minute

// IsFull returns whether the pack is ready to save (large enough or old enough)
func (p *Packer) IsFull(maxPackSize uint) bool {
	p.m.Lock()
	defer p.m.Unlock()

	if p.bytes >= maxPackSize {
		return true
	}

	age := time.Since(p.created)
	return age >= packMaxAge
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
func (p *Packer) Blobs() []restic.Blob {
	p.m.Lock()
	defer p.m.Unlock()

	return p.blobs
}

func (p *Packer) String() string {
	return fmt.Sprintf("<Packer %d blobs, %d bytes>", len(p.blobs), p.bytes)
}

var (
	// we require at least one entry in the header, and one blob for a pack file
	minFileSize = EntrySize + crypto.Extension + uint(headerLengthSize)
)

const (
	// size of the header-length field at the end of the file; it is a uint32
	headerLengthSize = 4
	// HeaderSize is the header's constant overhead (independent of #entries)
	HeaderSize = headerLengthSize + crypto.Extension

	maxHeaderSize = 16 * 1024 * 1024
	// number of header enries to download as part of header-length request
	eagerEntries = 15
)

// readRecords reads up to max records from the underlying ReaderAt, returning
// the raw header, the total number of records in the header, and any error.
// If the header contains fewer than max entries, the header is truncated to
// the appropriate size.
func readRecords(rd io.ReaderAt, size int64, max int) ([]byte, int, error) {
	var bufsize int
	bufsize += max * int(EntrySize)
	bufsize += crypto.Extension
	bufsize += headerLengthSize

	if bufsize > int(size) {
		bufsize = int(size)
	}

	b := make([]byte, bufsize)
	off := size - int64(bufsize)
	if _, err := rd.ReadAt(b, off); err != nil {
		return nil, 0, err
	}

	hlen := binary.LittleEndian.Uint32(b[len(b)-headerLengthSize:])
	b = b[:len(b)-headerLengthSize]
	debug.Log("header length: %v", hlen)

	var err error
	switch {
	case hlen == 0:
		err = InvalidFileError{Message: "header length is zero"}
	case hlen < crypto.Extension:
		err = InvalidFileError{Message: "header length is too small"}
	case (hlen-crypto.Extension)%uint32(EntrySize) != 0:
		err = InvalidFileError{Message: "header length is invalid"}
	case int64(hlen) > size-int64(headerLengthSize):
		err = InvalidFileError{Message: "header is larger than file"}
	case int64(hlen) > maxHeaderSize:
		err = InvalidFileError{Message: "header is larger than maxHeaderSize"}
	}
	if err != nil {
		return nil, 0, errors.Wrap(err, "readHeader")
	}

	total := (int(hlen) - crypto.Extension) / int(EntrySize)
	if total < max {
		// truncate to the beginning of the pack header
		b = b[len(b)-int(hlen):]
	}

	return b, total, nil
}

// readHeader reads the header at the end of rd. size is the length of the
// whole data accessible in rd.
func readHeader(rd io.ReaderAt, size int64) ([]byte, error) {
	debug.Log("size: %v", size)
	if size < int64(minFileSize) {
		err := InvalidFileError{Message: "file is too small"}
		return nil, errors.Wrap(err, "readHeader")
	}

	// assuming extra request is significantly slower than extra bytes download,
	// eagerly download eagerEntries header entries as part of header-length request.
	// only make second request if actual number of entries is greater than eagerEntries

	b, c, err := readRecords(rd, size, eagerEntries)
	if err != nil {
		return nil, err
	}
	if c <= eagerEntries {
		// eager read sufficed, return what we got
		return b, nil
	}
	b, _, err = readRecords(rd, size, c)
	if err != nil {
		return nil, err
	}
	return b, nil
}

// InvalidFileError is return when a file is found that is not a pack file.
type InvalidFileError struct {
	Message string
}

func (e InvalidFileError) Error() string {
	return e.Message
}

// List returns the list of entries found in a pack file and the length of the
// header (including header size and crypto overhead)
func List(k *crypto.Key, rd io.ReaderAt, size int64) (entries []restic.Blob, hdrSize uint32, err error) {
	buf, err := readHeader(rd, size)
	if err != nil {
		return nil, 0, err
	}

	if len(buf) < k.NonceSize()+k.Overhead() {
		return nil, 0, errors.New("invalid header, too small")
	}

	hdrSize = headerLengthSize + uint32(len(buf))

	nonce, buf := buf[:k.NonceSize()], buf[k.NonceSize():]
	buf, err = k.Open(buf[:0], nonce, buf, nil)
	if err != nil {
		return nil, 0, err
	}

	entries = make([]restic.Blob, 0, uint(len(buf))/EntrySize)

	pos := uint(0)
	for len(buf) > 0 {
		entry, err := parseHeaderEntry(buf)
		if err != nil {
			return nil, 0, err
		}
		entry.Offset = pos

		entries = append(entries, entry)
		pos += entry.Length
		buf = buf[EntrySize:]
	}

	return entries, hdrSize, nil
}

// PackedSizeOfBlob returns the size a blob actually uses when saved in a pack
func PackedSizeOfBlob(blobLength uint) uint {
	return blobLength + EntrySize
}

func parseHeaderEntry(p []byte) (b restic.Blob, err error) {
	if uint(len(p)) < EntrySize {
		err = errors.Errorf("parseHeaderEntry: buffer of size %d too short", len(p))
		return b, err
	}
	p = p[:EntrySize]

	switch p[0] {
	case 0:
		b.Type = restic.DataBlob
	case 1:
		b.Type = restic.TreeBlob
	default:
		return b, errors.Errorf("invalid type %d", p[0])
	}

	b.Length = uint(binary.LittleEndian.Uint32(p[1:5]))
	copy(b.ID[:], p[5:])

	return b, nil
}
