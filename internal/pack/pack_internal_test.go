package pack

import (
	"bytes"
	"encoding/binary"
	"io"
	"strings"
	"testing"

	"github.com/restic/restic/internal/crypto"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
)

func TestParseHeaderEntry(t *testing.T) {
	h := headerEntry{
		Type:   0, // Blob
		Length: 100,
	}
	for i := range h.ID {
		h.ID[i] = byte(i)
	}

	buf := new(bytes.Buffer)
	_ = binary.Write(buf, binary.LittleEndian, &h)

	b, size, err := parseHeaderEntry(buf.Bytes())
	rtest.OK(t, err)
	rtest.Equals(t, restic.DataBlob, b.Type)
	rtest.Equals(t, plainEntrySize, size)
	t.Logf("%v %v", h.ID, b.ID)
	rtest.Equals(t, h.ID[:], b.ID[:])
	rtest.Equals(t, uint(h.Length), b.Length)
	rtest.Equals(t, uint(0), b.UncompressedLength)

	c := compressedHeaderEntry{
		Type:               2, // compressed Blob
		Length:             100,
		UncompressedLength: 200,
	}
	for i := range c.ID {
		c.ID[i] = byte(i)
	}

	buf = new(bytes.Buffer)
	_ = binary.Write(buf, binary.LittleEndian, &c)

	b, size, err = parseHeaderEntry(buf.Bytes())
	rtest.OK(t, err)
	rtest.Equals(t, restic.DataBlob, b.Type)
	rtest.Equals(t, entrySize, size)
	t.Logf("%v %v", c.ID, b.ID)
	rtest.Equals(t, c.ID[:], b.ID[:])
	rtest.Equals(t, uint(c.Length), b.Length)
	rtest.Equals(t, uint(c.UncompressedLength), b.UncompressedLength)
}

func TestParseHeaderEntryErrors(t *testing.T) {
	h := headerEntry{
		Type:   0, // Blob
		Length: 100,
	}
	for i := range h.ID {
		h.ID[i] = byte(i)
	}

	h.Type = 0xae
	buf := new(bytes.Buffer)
	_ = binary.Write(buf, binary.LittleEndian, &h)

	_, _, err := parseHeaderEntry(buf.Bytes())
	rtest.Assert(t, err != nil, "no error for invalid type")

	h.Type = 0
	buf.Reset()
	_ = binary.Write(buf, binary.LittleEndian, &h)

	_, _, err = parseHeaderEntry(buf.Bytes()[:plainEntrySize-1])
	rtest.Assert(t, err != nil, "no error for short input")
}

type countingReaderAt struct {
	delegate        io.ReaderAt
	invocationCount int
}

func (rd *countingReaderAt) ReadAt(p []byte, off int64) (n int, err error) {
	rd.invocationCount++
	return rd.delegate.ReadAt(p, off)
}

func TestReadHeaderEagerLoad(t *testing.T) {

	testReadHeader := func(dataSize, entryCount, expectedReadInvocationCount int) {
		expectedHeader := rtest.Random(0, entryCount*int(entrySize)+crypto.Extension)

		buf := &bytes.Buffer{}
		buf.Write(rtest.Random(0, dataSize))                                             // pack blobs data
		buf.Write(expectedHeader)                                                        // pack header
		rtest.OK(t, binary.Write(buf, binary.LittleEndian, uint32(len(expectedHeader)))) // pack header length

		rd := &countingReaderAt{delegate: bytes.NewReader(buf.Bytes())}

		header, err := readHeader(rd, int64(buf.Len()))
		rtest.OK(t, err)

		rtest.Equals(t, expectedHeader, header)
		rtest.Equals(t, expectedReadInvocationCount, rd.invocationCount)
	}

	// basic
	testReadHeader(100, 1, 1)

	// header entries == eager entries
	testReadHeader(100, eagerEntries-1, 1)
	testReadHeader(100, eagerEntries, 1)
	testReadHeader(100, eagerEntries+1, 2)

	// file size == eager header load size
	eagerLoadSize := int((eagerEntries * entrySize) + crypto.Extension)
	headerSize := int(1*entrySize) + crypto.Extension
	dataSize := eagerLoadSize - headerSize - binary.Size(uint32(0))
	testReadHeader(dataSize-1, 1, 1)
	testReadHeader(dataSize, 1, 1)
	testReadHeader(dataSize+1, 1, 1)
	testReadHeader(dataSize+2, 1, 1)
	testReadHeader(dataSize+3, 1, 1)
	testReadHeader(dataSize+4, 1, 1)
}

func TestReadRecords(t *testing.T) {
	testReadRecords := func(dataSize, entryCount, totalRecords int) {
		totalHeader := rtest.Random(0, totalRecords*int(entrySize)+crypto.Extension)
		bufSize := entryCount*int(entrySize) + crypto.Extension
		off := len(totalHeader) - bufSize
		if off < 0 {
			off = 0
		}
		expectedHeader := totalHeader[off:]

		buf := &bytes.Buffer{}
		buf.Write(rtest.Random(0, dataSize))                                          // pack blobs data
		buf.Write(totalHeader)                                                        // pack header
		rtest.OK(t, binary.Write(buf, binary.LittleEndian, uint32(len(totalHeader)))) // pack header length

		rd := bytes.NewReader(buf.Bytes())

		header, count, err := readRecords(rd, int64(rd.Len()), bufSize+4)
		rtest.OK(t, err)
		rtest.Equals(t, len(totalHeader)+4, count)
		rtest.Equals(t, expectedHeader, header)
	}

	// basic
	testReadRecords(100, 1, 1)
	testReadRecords(100, 0, 1)
	testReadRecords(100, 1, 0)

	// header entries ~ eager entries
	testReadRecords(100, eagerEntries, eagerEntries-1)
	testReadRecords(100, eagerEntries, eagerEntries)
	testReadRecords(100, eagerEntries, eagerEntries+1)

	// file size == eager header load size
	eagerLoadSize := int((eagerEntries * entrySize) + crypto.Extension)
	headerSize := int(1*entrySize) + crypto.Extension
	dataSize := eagerLoadSize - headerSize - binary.Size(uint32(0))
	testReadRecords(dataSize-1, 1, 1)
	testReadRecords(dataSize, 1, 1)
	testReadRecords(dataSize+1, 1, 1)
	testReadRecords(dataSize+2, 1, 1)
	testReadRecords(dataSize+3, 1, 1)
	testReadRecords(dataSize+4, 1, 1)

	for i := 0; i < 2; i++ {
		for j := 0; j < 2; j++ {
			testReadRecords(dataSize, i, j)
		}
	}
}

func TestUnpackedVerification(t *testing.T) {
	// create random keys
	k := crypto.NewRandomKey()
	blobs := []restic.Blob{
		{
			BlobHandle:         restic.NewRandomBlobHandle(),
			Length:             42,
			Offset:             0,
			UncompressedLength: 2 * 42,
		},
	}

	type DamageType string
	const (
		damageData       DamageType = "data"
		damageCiphertext DamageType = "ciphertext"
		damageLength     DamageType = "length"
	)

	for _, test := range []struct {
		damage DamageType
		msg    string
	}{
		{"", ""},
		{damageData, "pack header entry mismatch"},
		{damageCiphertext, "ciphertext verification failed"},
		{damageLength, "header decoding failed"},
	} {
		header, err := makeHeader(blobs)
		rtest.OK(t, err)

		if test.damage == damageData {
			header[8] ^= 0x42
		}

		encryptedHeader := make([]byte, 0, crypto.CiphertextLength(len(header)))
		nonce := crypto.NewRandomNonce()
		encryptedHeader = append(encryptedHeader, nonce...)
		encryptedHeader = k.Seal(encryptedHeader, nonce, header, nil)
		encryptedHeader = binary.LittleEndian.AppendUint32(encryptedHeader, uint32(len(encryptedHeader)))

		if test.damage == damageCiphertext {
			encryptedHeader[8] ^= 0x42
		}
		if test.damage == damageLength {
			encryptedHeader[len(encryptedHeader)-1] ^= 0x42
		}

		err = verifyHeader(k, encryptedHeader, blobs)
		if test.msg == "" {
			rtest.Assert(t, err == nil, "expected no error, got %v", err)
		} else {
			rtest.Assert(t, strings.Contains(err.Error(), test.msg), "expected error to contain %q, got %q", test.msg, err)
		}
	}
}
