package pack

import (
	"bytes"
	"encoding/binary"
	"io"
	"testing"

	"github.com/restic/restic/internal/crypto"
	rtest "github.com/restic/restic/internal/test"
)

type countingReaderAt struct {
	delegate        io.ReaderAt
	invocationCount int
}

func (rd *countingReaderAt) ReadAt(p []byte, off int64) (n int, err error) {
	rd.invocationCount++
	return rd.delegate.ReadAt(p, off)
}

func TestReadHeaderEagerLoad(t *testing.T) {

	testReadHeader := func(entryCount uint, expectedReadInvocationCount int) {
		expectedHeader := rtest.Random(0, int(entryCount*entrySize)+crypto.Extension)

		buf := &bytes.Buffer{}
		buf.Write(rtest.Random(0, 100))                                     // pack blobs data
		buf.Write(expectedHeader)                                           // pack header
		binary.Write(buf, binary.LittleEndian, uint32(len(expectedHeader))) // pack header length

		rd := &countingReaderAt{delegate: bytes.NewReader(buf.Bytes())}

		header, err := readHeader(rd, int64(buf.Len()))
		rtest.OK(t, err)

		rtest.Equals(t, expectedHeader, header)
		rtest.Equals(t, expectedReadInvocationCount, rd.invocationCount)
	}

	testReadHeader(1, 1)
	testReadHeader(eagerEntries-1, 1)
	testReadHeader(eagerEntries, 1)
	testReadHeader(eagerEntries+1, 2)
}
