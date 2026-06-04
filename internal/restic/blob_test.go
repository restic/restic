package restic

import (
	"encoding/json"
	"testing"
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
