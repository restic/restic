package index_test

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/restic/restic/internal/repository/index"
	"github.com/restic/restic/internal/repository/pack"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
)

func mustDecodeIndex(t *testing.T, doc string) *index.Index {
	t.Helper()
	idx, err := index.DecodeIndex([]byte(doc), restic.NewRandomID())
	rtest.OK(t, err)
	return idx
}

func assertDecodeIndexError(t *testing.T, doc string) {
	t.Helper()
	_, err := index.DecodeIndex([]byte(doc), restic.NewRandomID())
	if err == nil {
		t.Fatalf("expected error for %s", doc)
	}
}

func sampleDecodeIndex() *index.Index {
	idx := index.NewIndex()
	idx.StorePack(restic.TestParseID("73d04e6125cf3c28a299cc2f3cca3b78ceac396e4fcf9575e34536b26782413c"), pack.Blobs{
		{
			BlobHandle: restic.BlobHandle{
				Type: restic.DataBlob,
				ID:   restic.TestParseID("3ec79977ef0cf5de7b08cd12b874cd0f62bbaf7f07f3497a5b1bbcc8cb39b1ce"),
			},
			Offset: 0,
			Length: 10,
		},
		{
			BlobHandle: restic.BlobHandle{
				Type: restic.TreeBlob,
				ID:   restic.TestParseID("9ccb846e60d90d4eb915848add7aa7ea1e4bbabfc60e573db9f7bfb2789afbae"),
			},
			Offset:             10,
			Length:             20,
			UncompressedLength: 40,
		},
	})
	idx.StorePack(restic.TestParseID("ed54ae36197f4745ebc4b54d10e0f623eaaaedd03013eb7ae90df881b7781452"), pack.Blobs{
		{
			BlobHandle: restic.BlobHandle{
				Type: restic.DataBlob,
				ID:   restic.TestParseID("d3dc577b4ffd38cc4b32122cabf8655a0223ed22edfd93b353dc0c3f2b0fdf66"),
			},
			Offset:             0,
			Length:             5,
			UncompressedLength: 9,
		},
	})
	return idx
}

func TestDecodeIndexRejectsMalformed(t *testing.T) {
	tests := []struct {
		name string
		doc  string
	}{
		{"empty input", ""},
		{"truncated object", "{"},
		{"truncated packs array", `{"packs":[`},
		{"truncated pack", `{"packs":[{`},
		{"top-level array", "[]"},
		{"packs is an object", `{"packs":{}}`},
		{"pack is an array", `{"packs":[[]]}`},
		{"blobs is an object", `{"packs":[{"id":"73d04e6125cf3c28a299cc2f3cca3b78ceac396e4fcf9575e34536b26782413c","blobs":{}}]}`},
		{"trailing object", `{"packs":[]}{}`},
		{"trailing junk", `{"packs":[]} trailing`},
		{"truncated blob", `{"packs":[{"id":"73d04e6125cf3c28a299cc2f3cca3b78ceac396e4fcf9575e34536b26782413c","blobs":[{}]}`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertDecodeIndexError(t, test.doc)
		})
	}
}

func TestDecodeIndexRejectsBadFields(t *testing.T) {
	const blob = "3ec79977ef0cf5de7b08cd12b874cd0f62bbaf7f07f3497a5b1bbcc8cb39b1ce"
	blobDoc := func(id, typ, offset, length, extra string) string {
		return fmt.Sprintf(`{"packs":[{"id":"73d04e6125cf3c28a299cc2f3cca3b78ceac396e4fcf9575e34536b26782413c","blobs":[{"id":%s,"type":%s,"offset":%s,"length":%s%s}]}]}`,
			id, typ, offset, length, extra)
	}
	quotedBlob := `"` + blob + `"`

	tests := []struct {
		name string
		doc  string
	}{
		{"short blob id", blobDoc(`"abcd"`, `"data"`, "0", "1", "")},
		{"non-hex blob id", blobDoc(`"`+strings.Repeat("g", 64)+`"`, `"data"`, "0", "1", "")},
		{"blob id is a number", blobDoc("1", `"data"`, "0", "1", "")},
		{"whitespace in blob id", blobDoc(`" `+blob+`"`, `"data"`, "0", "1", "")},
		{"unknown blob type", blobDoc(quotedBlob, `"foo"`, "0", "1", "")},
		{"blob type is a number", blobDoc(quotedBlob, "1", "0", "1", "")},
		{"offset is a string", blobDoc(quotedBlob, `"data"`, `"0"`, "1", "")},
		{"negative offset", blobDoc(quotedBlob, `"data"`, "-1", "1", "")},
		{"fractional offset", blobDoc(quotedBlob, `"data"`, "1.5", "1", "")},
		{"length is a string", blobDoc(quotedBlob, `"data"`, "0", `"1"`, "")},
		{"negative length", blobDoc(quotedBlob, `"data"`, "0", "-1", "")},
		{"fractional length", blobDoc(quotedBlob, `"data"`, "0", "1.5", "")},
		{"uncompressed_length is a string", blobDoc(quotedBlob, `"data"`, "0", "1", `,"uncompressed_length":"1"`)},
		{"negative uncompressed_length", blobDoc(quotedBlob, `"data"`, "0", "1", `,"uncompressed_length":-1`)},
		{"fractional uncompressed_length", blobDoc(quotedBlob, `"data"`, "0", "1", `,"uncompressed_length":1.5`)},
		{"short pack id", fmt.Sprintf(`{"packs":[{"id":"abcd","blobs":[{"id":"%s","type":"data","offset":0,"length":1}]}]}`, blob)},
		{"pack id is a number", fmt.Sprintf(`{"packs":[{"id":1,"blobs":[{"id":"%s","type":"data","offset":0,"length":1}]}]}`, blob)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertDecodeIndexError(t, test.doc)
		})
	}
}

func TestDecodeIndexUnknownKeys(t *testing.T) {
	doc := `{
		"supersedes": ["ed54ae36197f4745ebc4b54d10e0f623eaaaedd03013eb7ae90df881b7781452"],
		"extra": {"nested": [1, true, null, {"a": "b"}]},
		"packs": [{
			"comment": "hello",
			"id": "73d04e6125cf3c28a299cc2f3cca3b78ceac396e4fcf9575e34536b26782413c",
			"blobs": [{
				"note": [1, {"a": 2}],
				"id": "3ec79977ef0cf5de7b08cd12b874cd0f62bbaf7f07f3497a5b1bbcc8cb39b1ce",
				"type": "data",
				"offset": 7,
				"length": 8,
				"uncompressed_length": 9
			}]
		}]
	}`

	idx := mustDecodeIndex(t, doc)
	packID := restic.TestParseID("73d04e6125cf3c28a299cc2f3cca3b78ceac396e4fcf9575e34536b26782413c")
	blobID := restic.TestParseID("3ec79977ef0cf5de7b08cd12b874cd0f62bbaf7f07f3497a5b1bbcc8cb39b1ce")
	rtest.Equals(t, restic.NewIDSet(packID), idx.Packs())
	rtest.Equals(t, uint(1), idx.Len(restic.DataBlob))
	rtest.Equals(t, uint(0), idx.Len(restic.TreeBlob))

	list := idx.Lookup(restic.BlobHandle{ID: blobID, Type: restic.DataBlob}, nil)
	rtest.Equals(t, 1, len(list))
	rtest.Equals(t, uint(7), list[0].Blob.Offset)
	rtest.Equals(t, uint(8), list[0].Blob.Length)
	rtest.Equals(t, uint(9), list[0].Blob.UncompressedLength)
	rtest.Equals(t, packID, list[0].PackID())
}

func TestDecodeIndexEmpty(t *testing.T) {
	for _, doc := range []string{`{}`, `{"packs":[]}`, "{\"packs\":[]}\n\t "} {
		t.Run(doc, func(t *testing.T) {
			idx := mustDecodeIndex(t, doc)
			rtest.Equals(t, 0, len(idx.Packs()))
			rtest.Equals(t, uint(0), idx.Len(restic.DataBlob))
			rtest.Equals(t, uint(0), idx.Len(restic.TreeBlob))
		})
	}

	idx := mustDecodeIndex(t, `{"packs":[{"id":"73d04e6125cf3c28a299cc2f3cca3b78ceac396e4fcf9575e34536b26782413c","blobs":[]}]}`)
	rtest.Equals(t, restic.NewIDSet(restic.TestParseID("73d04e6125cf3c28a299cc2f3cca3b78ceac396e4fcf9575e34536b26782413c")), idx.Packs())
	rtest.Equals(t, uint(0), idx.Len(restic.DataBlob))
	rtest.Equals(t, uint(0), idx.Len(restic.TreeBlob))
}

func TestDecodeIndexRoundtrip(t *testing.T) {
	idx := sampleDecodeIndex()

	var buf bytes.Buffer
	rtest.OK(t, idx.Encode(&buf))
	decoded := mustDecodeIndex(t, buf.String())
	rtest.Equals(t, packedBlobs(idx), packedBlobs(decoded))
}

func packedBlobs(idx *index.Index) map[pack.PackedBlob]struct{} {
	got := make(map[pack.PackedBlob]struct{})
	for pb := range idx.Values() {
		got[*pb] = struct{}{}
	}
	return got
}
