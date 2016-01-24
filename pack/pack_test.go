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

	"github.com/restic/restic/backend"
	"github.com/restic/restic/crypto"
	"github.com/restic/restic/pack"
	. "github.com/restic/restic/test"
)

var lengths = []int{23, 31650, 25860, 10928, 13769, 19862, 5211, 127, 13690, 30231}

func TestCreatePack(t *testing.T) {
	type Buf struct {
		data []byte
		id   backend.ID
	}

	bufs := []Buf{}

	for _, l := range lengths {
		b := make([]byte, l)
		_, err := io.ReadFull(rand.Reader, b)
		OK(t, err)
		h := sha256.Sum256(b)
		bufs = append(bufs, Buf{data: b, id: h})
	}

	// create random keys
	k := crypto.NewRandomKey()

	// pack blobs
	p := pack.NewPacker(k, nil)
	for _, b := range bufs {
		p.Add(pack.Tree, b.id, bytes.NewReader(b.data))
	}

	packData, err := p.Finalize()
	OK(t, err)

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
	Equals(t, written, len(packData))
	Equals(t, uint(written), p.Size())

	// read and parse it again
	rd := bytes.NewReader(packData)
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
