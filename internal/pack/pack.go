package pack

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"sync"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/restic"

	"github.com/restic/restic/internal/crypto"
)

// There are two types of Packer file format:
// 1. Lagacy format:
//
// 2. New format - headers vary in length.

const (
	packerHeaderLegacyType uint32 = iota
	packerHeaderVersion1Type
)

// Packer is used to create a new Pack.
type Packer struct {
	blobs []restic.Blob

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
func (p *Packer) Add(t restic.BlobType, id restic.ID,
	data []byte, actual_length uint) (int, error) {
	p.m.Lock()
	defer p.m.Unlock()

	c := restic.Blob{Type: t, ID: id}

	debug.Log("%v: Writing blob %v @ offset %v and length %v",
		p, id, p.bytes, len(data))

	n, err := p.wr.Write(data)
	c.ActualLength = actual_length
	c.PackedLength = uint(n)
	c.Offset = p.bytes
	p.bytes += uint(n)
	p.blobs = append(p.blobs, c)

	return n, errors.Wrap(err, "Write")
}

var entrySizeLegacy = uint(binary.Size(restic.BlobType(0)) +
	binary.Size(uint32(0)) +
	len(restic.ID{}))

// headerEntryLegacy is used with encoding/binary to read and write
// uncompressed header entries
type headerEntryLegacy struct {
	Length uint32
	ID     restic.ID
}

var entrySizeZlib = uint(binary.Size(restic.BlobType(0)) +
	binary.Size(uint32(0)) +
	binary.Size(uint32(0)) +
	len(restic.ID{}))

// headerEntryZlib is used with encoding/binary to read and write zlib
// header entries
type headerEntryZlib struct {
	ActualLength uint32
	PackedLength uint32
	ID           restic.ID
}

// This header resides behind the final uint32
type packerHeader struct {
	TotalBytes   uint32
	TotalRecords uint32

	// Last member must be the type to maintain alignment with the
	// legacy format.
	Type    uint8 // Must be packerHeaderVersion1Type
	padding [3]uint8
}

const packerHeaderSize uint = 4 + 4 + 1 + 3

// Finalize writes the header for all added blobs and finalizes the pack.
// Returned are the number of bytes written, including the header. If the
// underlying writer implements io.Closer, it is closed.
func (p *Packer) Finalize() (uint, error) {
	p.m.Lock()
	defer p.m.Unlock()

	// Write all the records to a buffer then encrypt the entire
	// buffer.
	hdrBuf := bytes.NewBuffer(nil)
	bytesHeader, err := p.writeHeader(hdrBuf)
	if err != nil {
		return 0, err
	}

	encryptedHeader := make([]byte, 0, hdrBuf.Len()+p.k.Overhead()+p.k.NonceSize())
	nonce := crypto.NewRandomNonce()
	encryptedHeader = append(encryptedHeader, nonce...)
	encryptedHeader = p.k.Seal(encryptedHeader, nonce, hdrBuf.Bytes(), nil)

	// append the encrypted records
	n, err := p.wr.Write(encryptedHeader)
	if err != nil {
		return 0, errors.Wrap(err, "Write")
	}

	bytesWritten := uint(restic.CiphertextLength(int(bytesHeader)))
	if n != int(bytesWritten) {
		return 0, errors.New(fmt.Sprintf(
			"wrong number of bytes written: %v expecting %v",
			n, bytesWritten))
	}

	// append the packer header
	packer_header := packerHeader{
		TotalBytes:   uint32(bytesWritten),
		TotalRecords: uint32(len(p.blobs)),
		Type:         uint8(packerHeaderVersion1Type),
	}

	err = binary.Write(p.wr, binary.LittleEndian, packer_header)
	if err != nil {
		return bytesWritten, errors.Wrap(err, "binary.Write")
	}

	bytesWritten += packerHeaderSize

	p.bytes = uint(bytesWritten)

	if w, ok := p.wr.(io.Closer); ok {
		return bytesWritten, w.Close()
	}

	return bytesWritten, nil
}

// writeHeader constructs and writes the header to wr.
func (p *Packer) writeHeader(wr io.Writer) (bytesWritten uint, err error) {
	for _, b := range p.blobs {
		err := binary.Write(wr, binary.LittleEndian, b.Type)
		if err != nil {
			return bytesWritten, errors.Wrap(err, "binary.Write")
		}

		switch b.Type {
		case restic.DataBlob, restic.TreeBlob:
			entry := headerEntryLegacy{
				Length: uint32(b.ActualLength),
				ID:     b.ID,
			}

			err := binary.Write(wr, binary.LittleEndian, entry)
			if err != nil {
				return bytesWritten, errors.Wrap(err, "binary.Write")
			}

			bytesWritten += entrySizeLegacy

		case restic.ZlibBlob:
			entry := headerEntryZlib{
				ActualLength: uint32(b.ActualLength),
				PackedLength: uint32(b.PackedLength),
				ID:           b.ID,
			}

			err := binary.Write(wr, binary.LittleEndian, entry)
			if err != nil {
				return bytesWritten, errors.Wrap(err, "binary.Write")
			}

			bytesWritten += entrySizeZlib

		default:
			return 0, errors.Errorf("invalid blob type %v", b.Type)
		}
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
func (p *Packer) Blobs() []restic.Blob {
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

var (
	// size of the header-length field at the end of the file
	headerLengthSize = binary.Size(uint32(0))

	// we require at least one entry in the header, and one blob for a pack file
	minFileSize = entrySizeLegacy + crypto.Extension + uint(headerLengthSize)
)

const (
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

	// entrySizeZlib is largest right now.
	bufsize += max * int(entrySizeZlib)
	bufsize += crypto.Extension
	bufsize += headerLengthSize

	if bufsize > int(size) {
		bufsize = int(size)
	}

	b := make([]byte, bufsize)
	off := size - int64(bufsize)
	n, err := rd.ReadAt(b, off)
	if err != nil {
		return nil, 0, err
	}

	if n < bufsize {
		return nil, 0, errors.New("Short read")
	}

	tail_sig := binary.LittleEndian.Uint32(b[len(b)-headerLengthSize:])
	header_type := tail_sig & 0xFF000000

	// This is a Legacy header
	switch header_type {
	case packerHeaderLegacyType:
		hlen := tail_sig
		b = b[:len(b)-headerLengthSize]
		debug.Log("header length: %v", hlen)

		var err error
		switch {
		case hlen == 0:
			err = InvalidFileError{Message: "header length is zero"}
		case hlen < crypto.Extension:
			err = InvalidFileError{Message: "header length is too small"}
		case (hlen-crypto.Extension)%uint32(entrySizeLegacy) != 0:
			err = InvalidFileError{Message: "header length is invalid"}
		case int64(hlen) > size-int64(headerLengthSize):
			err = InvalidFileError{Message: "header is larger than file"}
		case int64(hlen) > maxHeaderSize:
			err = InvalidFileError{Message: "header is larger than maxHeaderSize"}
		}
		if err != nil {
			return nil, 0, errors.Wrap(err, "readHeader")
		}

		// Legacy header all records are same size and total
		// count is calculated by the total size of the index.
		total := (int(hlen) - crypto.Extension) / int(entrySizeLegacy)
		if total < max {
			// truncate to the beginning of the pack header
			b = b[len(b)-int(hlen):]
		}
		return b, total, nil

	case packerHeaderVersion1Type:
		//FIXME

	}
	return nil, 0, errors.New("Unsupported packer file format")
}

// readHeader reads the header at the end of rd. size is the length of the
// whole data accessible in rd.
func readHeader(rd io.ReaderAt, size int64) ([]byte, int, error) {
	debug.Log("size: %v", size)
	if size < int64(minFileSize) {
		err := InvalidFileError{Message: "file is too small"}
		return nil, 0, errors.Wrap(err, "readHeader")
	}

	// assuming extra request is significantly slower than extra bytes download,
	// eagerly download eagerEntries header entries as part of header-length request.
	// only make second request if actual number of entries is greater than eagerEntries

	b, c, err := readRecords(rd, size, eagerEntries)
	if err != nil {
		return nil, 0, err
	}
	if c <= eagerEntries {
		// eager read sufficed, return what we got
		return b, c, nil
	}
	b, c, err = readRecords(rd, size, c)
	if err != nil {
		return nil, 0, err
	}
	return b, c, nil
}

// InvalidFileError is return when a file is found that is not a pack file.
type InvalidFileError struct {
	Message string
}

func (e InvalidFileError) Error() string {
	return e.Message
}

// List returns the list of entries found in a pack file.
func List(k *crypto.Key, rd io.ReaderAt, size int64) (entries []restic.Blob, err error) {
	buf, count, err := readHeader(rd, size)
	if err != nil {
		return nil, err
	}

	if len(buf) < k.NonceSize()+k.Overhead() {
		return nil, errors.New("invalid header, too small")
	}

	nonce, buf := buf[:k.NonceSize()], buf[k.NonceSize():]
	buf, err = k.Open(buf[:0], nonce, buf, nil)
	if err != nil {
		return nil, err
	}

	hdrRd := bytes.NewReader(buf)

	// Preallocate enough space.
	entries = make([]restic.Blob, 0, count)

	// Parse records into blobs
	for pos := 0; pos < len(buf); {
		var entry_type restic.BlobType

		err = binary.Read(hdrRd, binary.LittleEndian, &entry_type)
		if errors.Cause(err) == io.EOF {
			break
		}

		switch entry_type {
		case restic.DataBlob, restic.TreeBlob:
			record := headerEntryLegacy{}
			err = binary.Read(hdrRd, binary.LittleEndian, &record)
			if errors.Cause(err) == io.EOF {
				break
			}

			if err != nil {
				return nil, errors.Wrap(err, "binary.Read")
			}

			entry := restic.Blob{
				ActualLength:    uint(record.Length),
				PackedLength:    uint(record.Length),
				CompressionType: restic.CompressionTypeStored,
				ID:              record.ID,
				Offset:          uint(pos),
			}
			entries = append(entries, entry)
			pos += int(record.Length)

		case restic.ZlibBlob:
			record := headerEntryZlib{}
			err = binary.Read(hdrRd, binary.LittleEndian, &record)
			if errors.Cause(err) == io.EOF {
				break
			}

			if err != nil {
				return nil, errors.Wrap(err, "binary.Read")
			}

			entry := restic.Blob{
				ActualLength:    uint(record.ActualLength),
				PackedLength:    uint(record.PackedLength),
				CompressionType: restic.CompressionTypeZlib,
				ID:              record.ID,
				Offset:          uint(pos),
			}
			entries = append(entries, entry)
			pos += int(entry.PackedLength)

		default:
			return nil, errors.Errorf("invalid type %d", entry_type)
		}
	}

	return entries, nil
}
