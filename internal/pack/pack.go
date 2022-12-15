package pack

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"sync"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/restic"

	"github.com/restic/restic/internal/crypto"
)

// Packer is used to create a new Pack.
type Packer struct {
	blobs []restic.Blob

	bytes uint
	k     *crypto.Key
	wr    io.Writer

	m sync.Mutex
}

// NewPacker returns a new Packer that can be used to pack blobs together.
func NewPacker(k *crypto.Key, wr io.Writer) *Packer {
	return &Packer{k: k, wr: wr}
}

// Add saves the data read from rd as a new blob to the packer. Returned is the
// number of bytes written to the pack plus the pack header entry size.
func (p *Packer) Add(t restic.BlobType, id restic.ID, data []byte, uncompressedLength int) (int, error) {
	p.m.Lock()
	defer p.m.Unlock()

	c := restic.Blob{BlobHandle: restic.BlobHandle{Type: t, ID: id}}

	n, err := p.wr.Write(data)
	c.Length = uint(n)
	c.Offset = p.bytes
	c.UncompressedLength = uint(uncompressedLength)
	p.bytes += uint(n)
	p.blobs = append(p.blobs, c)
	n += CalculateEntrySize(c)

	return n, errors.Wrap(err, "Write")
}

var entrySize = uint(binary.Size(restic.BlobType(0)) + 2*headerLengthSize + len(restic.ID{}))
var plainEntrySize = uint(binary.Size(restic.BlobType(0)) + headerLengthSize + len(restic.ID{}))

// headerEntry describes the format of header entries. It serves only as
// documentation.
type headerEntry struct {
	Type   uint8
	Length uint32
	ID     restic.ID
}

// compressedHeaderEntry describes the format of header entries for compressed blobs.
// It serves only as documentation.
type compressedHeaderEntry struct {
	Type               uint8
	Length             uint32
	UncompressedLength uint32
	ID                 restic.ID
}

// Finalize writes the header for all added blobs and finalizes the pack.
func (p *Packer) Finalize() error {
	p.m.Lock()
	defer p.m.Unlock()

	header, err := p.makeHeader()
	if err != nil {
		return err
	}

	encryptedHeader := make([]byte, 0, crypto.CiphertextLength(len(header)))
	nonce := crypto.NewRandomNonce()
	encryptedHeader = append(encryptedHeader, nonce...)
	encryptedHeader = p.k.Seal(encryptedHeader, nonce, header, nil)

	// append the header
	n, err := p.wr.Write(encryptedHeader)
	if err != nil {
		return errors.Wrap(err, "Write")
	}

	hdrBytes := len(encryptedHeader)
	if n != hdrBytes {
		return errors.New("wrong number of bytes written")
	}

	// write length
	err = binary.Write(p.wr, binary.LittleEndian, uint32(hdrBytes))
	if err != nil {
		return errors.Wrap(err, "binary.Write")
	}
	p.bytes += uint(hdrBytes + binary.Size(uint32(0)))

	return nil
}

// HeaderOverhead returns an estimate of the number of bytes written by a call to Finalize.
func (p *Packer) HeaderOverhead() int {
	return crypto.CiphertextLength(0) + binary.Size(uint32(0))
}

// makeHeader constructs the header for p.
func (p *Packer) makeHeader() ([]byte, error) {
	buf := make([]byte, 0, len(p.blobs)*int(entrySize))

	for _, b := range p.blobs {
		switch {
		case b.Type == restic.DataBlob && b.UncompressedLength == 0:
			buf = append(buf, 0)
		case b.Type == restic.TreeBlob && b.UncompressedLength == 0:
			buf = append(buf, 1)
		case b.Type == restic.DataBlob && b.UncompressedLength != 0:
			buf = append(buf, 2)
		case b.Type == restic.TreeBlob && b.UncompressedLength != 0:
			buf = append(buf, 3)
		default:
			return nil, errors.Errorf("invalid blob type %v", b.Type)
		}

		var lenLE [4]byte
		binary.LittleEndian.PutUint32(lenLE[:], uint32(b.Length))
		buf = append(buf, lenLE[:]...)
		if b.UncompressedLength != 0 {
			binary.LittleEndian.PutUint32(lenLE[:], uint32(b.UncompressedLength))
			buf = append(buf, lenLE[:]...)
		}
		buf = append(buf, b.ID[:]...)
	}

	return buf, nil
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

// HeaderFull returns true if the pack header is full.
func (p *Packer) HeaderFull() bool {
	p.m.Lock()
	defer p.m.Unlock()
	return headerSize+uint(len(p.blobs)+1)*entrySize > MaxHeaderSize
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
	minFileSize = plainEntrySize + crypto.Extension + uint(headerLengthSize)
)

const (
	// size of the header-length field at the end of the file; it is a uint32
	headerLengthSize = 4
	// headerSize is the header's constant overhead (independent of #entries)
	headerSize = headerLengthSize + crypto.Extension

	// MaxHeaderSize is the max size of header including header-length field
	MaxHeaderSize = 16*1024*1024 + headerLengthSize
	// number of header enries to download as part of header-length request
	eagerEntries = 15
)

// readRecords reads up to bufsize bytes from the underlying ReaderAt, returning
// the raw header, the total number of bytes in the header, and any error.
// If the header contains fewer than bufsize bytes, the header is truncated to
// the appropriate size.
func readRecords(rd io.ReaderAt, size int64, bufsize int) ([]byte, int, error) {
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
	case int64(hlen) > size-int64(headerLengthSize):
		err = InvalidFileError{Message: "header is larger than file"}
	case int64(hlen) > MaxHeaderSize-int64(headerLengthSize):
		err = InvalidFileError{Message: "header is larger than maxHeaderSize"}
	}
	if err != nil {
		return nil, 0, errors.Wrap(err, "readHeader")
	}

	total := int(hlen + headerLengthSize)
	if total < bufsize {
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

	eagerSize := eagerEntries*int(entrySize) + headerSize
	b, c, err := readRecords(rd, size, eagerSize)
	if err != nil {
		return nil, err
	}
	if c <= eagerSize {
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

	if len(buf) < crypto.CiphertextLength(0) {
		return nil, 0, errors.New("invalid header, too small")
	}

	hdrSize = headerLengthSize + uint32(len(buf))

	nonce, buf := buf[:k.NonceSize()], buf[k.NonceSize():]
	buf, err = k.Open(buf[:0], nonce, buf, nil)
	if err != nil {
		return nil, 0, err
	}

	// might over allocate a bit if all blobs have EntrySize but only by a few percent
	entries = make([]restic.Blob, 0, uint(len(buf))/plainEntrySize)

	pos := uint(0)
	for len(buf) > 0 {
		entry, headerSize, err := parseHeaderEntry(buf)
		if err != nil {
			return nil, 0, err
		}
		entry.Offset = pos

		entries = append(entries, entry)
		pos += entry.Length
		buf = buf[headerSize:]
	}

	return entries, hdrSize, nil
}

func parseHeaderEntry(p []byte) (b restic.Blob, size uint, err error) {
	l := uint(len(p))
	size = plainEntrySize
	if l < plainEntrySize {
		err = errors.Errorf("parseHeaderEntry: buffer of size %d too short", len(p))
		return b, size, err
	}
	tpe := p[0]

	switch tpe {
	case 0, 2:
		b.Type = restic.DataBlob
	case 1, 3:
		b.Type = restic.TreeBlob
	default:
		return b, size, errors.Errorf("invalid type %d", tpe)
	}

	b.Length = uint(binary.LittleEndian.Uint32(p[1:5]))
	p = p[5:]
	if tpe == 2 || tpe == 3 {
		size = entrySize
		if l < entrySize {
			err = errors.Errorf("parseHeaderEntry: buffer of size %d too short", len(p))
			return b, size, err
		}
		b.UncompressedLength = uint(binary.LittleEndian.Uint32(p[0:4]))
		p = p[4:]
	}

	copy(b.ID[:], p[:])

	return b, size, nil
}

func CalculateEntrySize(blob restic.Blob) int {
	if blob.UncompressedLength != 0 {
		return int(entrySize)
	}
	return int(plainEntrySize)
}

func CalculateHeaderSize(blobs []restic.Blob) int {
	size := headerSize
	for _, blob := range blobs {
		size += CalculateEntrySize(blob)
	}
	return size
}

// Size returns the size of all packs computed by index information.
// If onlyHdr is set to true, only the size of the header is returned
// Note that this function only gives correct sizes, if there are no
// duplicates in the index.
func Size(ctx context.Context, mi restic.MasterIndex, onlyHdr bool) map[restic.ID]int64 {
	packSize := make(map[restic.ID]int64)

	mi.Each(ctx, func(blob restic.PackedBlob) {
		size, ok := packSize[blob.PackID]
		if !ok {
			size = headerSize
		}
		if !onlyHdr {
			size += int64(blob.Length)
		}
		packSize[blob.PackID] = size + int64(CalculateEntrySize(blob.Blob))
	})

	return packSize
}
