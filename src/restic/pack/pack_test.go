package pack_test

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"io"
	"io/ioutil"
	"testing"

	"restic/backend"
	"restic/backend/mem"
	"restic/crypto"
	"restic/pack"
	. "restic/test"
)

var lengths = []int{23, 31650, 25860, 10928, 13769, 19862, 5211, 127, 13690, 30231}

type Buf struct {
	data []byte
	id   backend.ID
}

func newPack(t testing.TB, k *crypto.Key) ([]Buf, []byte, uint) {
	bufs := []Buf{}

	for _, l := range lengths {
		b := make([]byte, l)
		_, err := io.ReadFull(rand.Reader, b)
		OK(t, err)
		h := sha256.Sum256(b)
		bufs = append(bufs, Buf{data: b, id: h})
	}

	// pack blobs
	p := pack.NewPacker(k, nil)
	for _, b := range bufs {
		p.Add(pack.Tree, b.id, b.data)
	}

	_, err := p.Finalize()
	OK(t, err)

	packData := p.Writer().(*bytes.Buffer).Bytes()
	return bufs, packData, p.Size()
}

func verifyBlobs(t testing.TB, bufs []Buf, k *crypto.Key, rd io.ReadSeeker, packSize uint) {
	written := 0
	for _, l := range lengths {
		written += l
	}
	// header length
	written += binary.Size(uint32(0))
	// header
	written += len(lengths) * (binary.Size(pack.BlobType(0)) + binary.Size(uint32(0)) + backend.IDSize)
	// header crypto
	written += crypto.Extension

	// check length
	Equals(t, uint(written), packSize)

	// read and parse it again
	np, err := pack.NewUnpacker(k, rd)
	OK(t, err)
	Equals(t, len(np.Entries), len(bufs))

	for i, b := range bufs {
		e := np.Entries[i]
		Equals(t, b.id, e.ID)

		brd, err := e.GetReader(rd)
		OK(t, err)
		data, err := ioutil.ReadAll(brd)
		OK(t, err)

		Assert(t, bytes.Equal(b.data, data),
			"data for blob %v doesn't match", i)
	}
}

func TestCreatePack(t *testing.T) {
	// create random keys
	k := crypto.NewRandomKey()

	bufs, packData, packSize := newPack(t, k)
	Equals(t, uint(len(packData)), packSize)
	verifyBlobs(t, bufs, k, bytes.NewReader(packData), packSize)
}

var blobTypeJSON = []struct {
	t   pack.BlobType
	res string
}{
	{pack.Data, `"data"`},
	{pack.Tree, `"tree"`},
}

func TestBlobTypeJSON(t *testing.T) {
	for _, test := range blobTypeJSON {
		// test serialize
		buf, err := json.Marshal(test.t)
		OK(t, err)
		Equals(t, test.res, string(buf))

		// test unserialize
		var v pack.BlobType
		err = json.Unmarshal([]byte(test.res), &v)
		OK(t, err)
		Equals(t, test.t, v)
	}
}

func TestUnpackReadSeeker(t *testing.T) {
	// create random keys
	k := crypto.NewRandomKey()

	bufs, packData, packSize := newPack(t, k)

	b := mem.New()
	id := backend.Hash(packData)

	handle := backend.Handle{Type: backend.Data, Name: id.String()}
	OK(t, b.Save(handle, packData))
	rd := backend.NewReadSeeker(b, handle)
	verifyBlobs(t, bufs, k, rd, packSize)
}
