package pack_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"io"
	"testing"

	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/backend/mem"
	"github.com/restic/restic/internal/crypto"
	"github.com/restic/restic/internal/repository/pack"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
)

var testLens = []int{23, 31650, 25860, 10928, 13769, 19862, 5211, 127, 13690, 30231}

type Buf struct {
	data []byte
	id   restic.ID
}

func newPack(t testing.TB, k *crypto.Key, lengths []int) ([]Buf, []byte, uint) {
	bufs := []Buf{}

	for _, l := range lengths {
		b := make([]byte, l)
		_, err := io.ReadFull(rand.Reader, b)
		rtest.OK(t, err)
		h := sha256.Sum256(b)
		bufs = append(bufs, Buf{data: b, id: h})
	}

	// pack blobs
	var buf bytes.Buffer
	p := pack.NewPacker(k, &buf)
	for _, b := range bufs {
		_, err := p.Add(restic.TreeBlob, b.id, b.data, 2*len(b.data))
		rtest.OK(t, err)
	}

	err := p.Finalize()
	rtest.OK(t, err)

	return bufs, buf.Bytes(), p.Size()
}

func verifyBlobs(t testing.TB, bufs []Buf, k *crypto.Key, rd io.ReaderAt, packSize uint) {
	written := 0
	for _, buf := range bufs {
		written += len(buf.data)
	}

	// read and parse it again
	entries, hdrSize, err := pack.List(k, rd, int64(packSize))
	rtest.OK(t, err)
	rtest.Equals(t, len(entries), len(bufs))

	// check the head size calculation for consistency
	headerSize := pack.CalculateHeaderSize(entries)
	written += headerSize

	// check length
	rtest.Equals(t, uint(written), packSize)
	rtest.Equals(t, headerSize, int(hdrSize))

	var buf []byte
	for i, b := range bufs {
		e := entries[i]
		rtest.Equals(t, b.id, e.ID)

		if len(buf) < int(e.Length) {
			buf = make([]byte, int(e.Length))
		}
		buf = buf[:int(e.Length)]
		n, err := rd.ReadAt(buf, int64(e.Offset))
		rtest.OK(t, err)
		buf = buf[:n]

		rtest.Assert(t, bytes.Equal(b.data, buf),
			"data for blob %v doesn't match", i)
	}
}

func TestCreatePack(t *testing.T) {
	// create random keys
	k := crypto.NewRandomKey()

	bufs, packData, packSize := newPack(t, k, testLens)
	rtest.Equals(t, uint(len(packData)), packSize)
	verifyBlobs(t, bufs, k, bytes.NewReader(packData), packSize)
}

var blobTypeJSON = []struct {
	t   restic.BlobType
	res string
}{
	{restic.DataBlob, `"data"`},
	{restic.TreeBlob, `"tree"`},
}

func TestBlobTypeJSON(t *testing.T) {
	for _, test := range blobTypeJSON {
		// test serialize
		buf, err := json.Marshal(test.t)
		rtest.OK(t, err)
		rtest.Equals(t, test.res, string(buf))

		// test unserialize
		var v restic.BlobType
		err = json.Unmarshal([]byte(test.res), &v)
		rtest.OK(t, err)
		rtest.Equals(t, test.t, v)
	}
}

func TestUnpackReadSeeker(t *testing.T) {
	// create random keys
	k := crypto.NewRandomKey()

	bufs, packData, packSize := newPack(t, k, testLens)

	b := mem.New()
	id := restic.Hash(packData)

	handle := backend.Handle{Type: backend.PackFile, Name: id.String()}
	rtest.OK(t, b.Save(context.TODO(), handle, backend.NewByteReader(packData, b.Hasher())))
	verifyBlobs(t, bufs, k, backend.ReaderAt(context.TODO(), b, handle), packSize)
}

func TestShortPack(t *testing.T) {
	k := crypto.NewRandomKey()

	bufs, packData, packSize := newPack(t, k, []int{23})

	b := mem.New()
	id := restic.Hash(packData)

	handle := backend.Handle{Type: backend.PackFile, Name: id.String()}
	rtest.OK(t, b.Save(context.TODO(), handle, backend.NewByteReader(packData, b.Hasher())))
	verifyBlobs(t, bufs, k, backend.ReaderAt(context.TODO(), b, handle), packSize)
}
