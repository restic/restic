package restic

import (
	"encoding/json"
	"testing"

	rtest "github.com/restic/restic/internal/test"
)

var blobTypeJSON = []struct {
	t   BlobType
	res string
}{
	{DataBlob, `"data"`},
	{TreeBlob, `"tree"`},
}

func TestBlobTypeJSON(t *testing.T) {
	for _, test := range blobTypeJSON {
		// test serialize
		buf, err := json.Marshal(test.t)
		if err != nil {
			t.Error(err)
			continue
		}
		if test.res != string(buf) {
			t.Errorf("want %q, got %q", test.res, string(buf))
			continue
		}

		// test unserialize
		var v BlobType
		err = json.Unmarshal([]byte(test.res), &v)
		if err != nil {
			t.Error(err)
			continue
		}
		if test.t != v {
			t.Errorf("want %v, got %v", test.t, v)
			continue
		}
	}
}

func TestBlobsSort(t *testing.T) {
	blobs := Blobs{
		{Offset: 100},
		{Offset: 0},
		{Offset: 50},
	}
	blobs.Sort()
	rtest.Equals(t, uint(0), blobs[0].Offset)
	rtest.Equals(t, uint(50), blobs[1].Offset)
	rtest.Equals(t, uint(100), blobs[2].Offset)
}
