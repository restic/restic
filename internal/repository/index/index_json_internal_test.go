package index

import (
	"fmt"
	"strings"
	"testing"

	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
)

// The fast parser is only ever allowed to differ from encoding/json by
// declining to parse. These tests pin that: for every input, either the fast
// parser bounces, or it produces exactly what encoding/json produces.

// The two index formats as documented in doc/040_backup.rst: v1 without
// compression, v2 with uncompressed_length. Both still carry "supersedes",
// which restic no longer writes but must still ignore on read.
var jsonExampleV1 = []byte(`
{
  "supersedes": [
	"ed54ae36197f4745ebc4b54d10e0f623eaaaedd03013eb7ae90df881b7781452"
  ],
  "packs": [
	{
	  "id": "73d04e6125cf3c28a299cc2f3cca3b78ceac396e4fcf9575e34536b26782413c",
	  "blobs": [
		{
		  "id": "3ec79977ef0cf5de7b08cd12b874cd0f62bbaf7f07f3497a5b1bbcc8cb39b1ce",
		  "type": "data",
		  "offset": 0,
		  "length": 38
		},{
		  "id": "9ccb846e60d90d4eb915848add7aa7ea1e4bbabfc60e573db9f7bfb2789afbae",
		  "type": "tree",
		  "offset": 38,
		  "length": 112
		}
	  ]
	}
  ]
}
`)

var jsonExampleV2 = []byte(`
{
  "packs": [
	{
	  "id": "73d04e6125cf3c28a299cc2f3cca3b78ceac396e4fcf9575e34536b26782413c",
	  "blobs": [
		{
		  "id": "3ec79977ef0cf5de7b08cd12b874cd0f62bbaf7f07f3497a5b1bbcc8cb39b1ce",
		  "type": "data",
		  "offset": 0,
		  "length": 38
		},
		{
		  "id": "9ccb846e60d90d4eb915848add7aa7ea1e4bbabfc60e573db9f7bfb2789afbae",
		  "type": "tree",
		  "offset": 38,
		  "length": 112,
		  "uncompressed_length": 511
		}
	  ]
	}
  ]
}
`)

// dump renders the complete observable state of an index as text, so two
// indexes can be compared including pack order and entry insertion order --
// neither of which idx.Each() exposes in a stable way.
func dump(idx *Index) string {
	var sb strings.Builder
	for _, packID := range idx.packs {
		fmt.Fprintf(&sb, "pack %v\n", packID)
	}
	for typ := range idx.byType {
		for e := range idx.byType[typ].values() {
			fmt.Fprintf(&sb, "blob type=%d id=%v pack=%d offset=%d length=%d ulength=%d\n",
				typ, e.id, e.packIndex, e.offset, e.length, e.uncompressedLength)
		}
	}
	return sb.String()
}

// decodeBoth runs both decoders over buf and returns their rendered results.
// A decoder that fails contributes its error text instead, so error behaviour
// is compared as strictly as success behaviour.
func decodeBoth(t testing.TB, buf []byte) (fast string, fastOK bool, reference string) {
	t.Helper()

	id := restic.TestParseID("1111111111111111111111111111111111111111111111111111111111111111")

	if idx, ok := decodeIndexFast(buf); ok {
		fast, fastOK = dump(idx), true
	}

	idx, err := decodeIndexJSON(buf, id)
	if err != nil {
		return fast, fastOK, "error: " + err.Error()
	}
	return fast, fastOK, dump(idx)
}

func TestDecodeIndexFastMatchesJSON(t *testing.T) {
	const (
		packA = "73d04e6125cf3c28a299cc2f3cca3b78ceac396e4fcf9575e34536b26782413c"
		blobA = "3ec79977ef0cf5de7b08cd12b874cd0f62bbaf7f07f3497a5b1bbcc8cb39b1ce"
		blobB = "9ccb846e60d90d4eb915848add7aa7ea1e4bbabfc60e573db9f7bfb2789afbae"
	)

	for _, test := range []struct {
		name string
		json string
		// mustParse asserts the fast path handles this input rather than
		// bouncing, so a regression that quietly routes everything to
		// encoding/json is caught.
		mustParse bool
	}{
		{name: "doc example v1", json: string(jsonExampleV1), mustParse: true},
		{name: "doc example v2", json: string(jsonExampleV2), mustParse: true},
		{
			name:      "empty object",
			json:      `{}`,
			mustParse: true,
		},
		{
			name:      "empty packs",
			json:      `{"packs":[]}`,
			mustParse: true,
		},
		{
			name:      "pack without blobs",
			json:      `{"packs":[{"id":"` + packA + `"}]}`,
			mustParse: true,
		},
		{
			name:      "pack with empty blob list",
			json:      `{"packs":[{"id":"` + packA + `","blobs":[]}]}`,
			mustParse: true,
		},
		{
			name:      "pack without id still reserves a slot",
			json:      `{"packs":[{"blobs":[{"id":"` + blobA + `","type":"data","offset":0,"length":1}]}]}`,
			mustParse: true,
		},
		{
			name:      "blobs before id",
			json:      `{"packs":[{"blobs":[{"id":"` + blobA + `","type":"tree","offset":0,"length":1}],"id":"` + packA + `"}]}`,
			mustParse: true,
		},
		{
			name:      "unknown fields are ignored",
			json:      `{"foo":{"a":[1,2,{"b":null}]},"packs":[{"zz":true,"id":"` + packA + `","blobs":[{"id":"` + blobA + `","type":"data","offset":0,"length":1,"extra":-1.5e3}]}],"bar":"x"}`,
			mustParse: true,
		},
		{
			name:      "supersedes is ignored",
			json:      `{"supersedes":["` + packA + `"],"packs":[]}`,
			mustParse: true,
		},
		{
			name:      "whitespace everywhere",
			json:      "{\n\t\"packs\" : [ {\r\n \"id\" : \"" + packA + "\" ,\n\"blobs\":[\t{ \"id\":\"" + blobA + "\",\"type\":\"data\",\"offset\":0,\"length\":38 } ]\n} ]\n}\n",
			mustParse: true,
		},
		{
			name:      "null packs",
			json:      `{"packs":null}`,
			mustParse: true,
		},
		{
			name:      "null blobs",
			json:      `{"packs":[{"id":"` + packA + `","blobs":null}]}`,
			mustParse: true,
		},
		{
			name:      "two packs sharing a blob id",
			json:      `{"packs":[{"id":"` + packA + `","blobs":[{"id":"` + blobA + `","type":"data","offset":0,"length":1}]},{"id":"` + packA + `","blobs":[{"id":"` + blobA + `","type":"data","offset":9,"length":2,"uncompressed_length":7}]}]}`,
			mustParse: true,
		},
		{
			name:      "missing type is the invalid blob type",
			json:      `{"packs":[{"id":"` + packA + `","blobs":[{"id":"` + blobB + `","offset":1,"length":2}]}]}`,
			mustParse: true,
		},

		// Inputs the fast parser is expected to hand off. Each must still
		// produce the reference result via the fallback.
		{name: "escaped key", json: `{"pack\u0073":[]}`},
		{name: "escaped blob type", json: `{"packs":[{"id":"` + packA + `","blobs":[{"id":"` + blobA + `","type":"dat\u0061","offset":0,"length":1}]}]}`},
		{name: "float length", json: `{"packs":[{"id":"` + packA + `","blobs":[{"id":"` + blobA + `","type":"data","offset":0,"length":1.0}]}]}`},
		{name: "exponent length", json: `{"packs":[{"id":"` + packA + `","blobs":[{"id":"` + blobA + `","type":"data","offset":0,"length":1e3}]}]}`},
		{name: "negative offset", json: `{"packs":[{"id":"` + packA + `","blobs":[{"id":"` + blobA + `","type":"data","offset":-1,"length":1}]}]}`},
		{name: "duplicate packs key", json: `{"packs":[],"packs":[]}`},
		{name: "duplicate blob length", json: `{"packs":[{"id":"` + packA + `","blobs":[{"id":"` + blobA + `","type":"data","offset":0,"length":1,"length":2}]}]}`},
		{name: "unknown blob type", json: `{"packs":[{"id":"` + packA + `","blobs":[{"id":"` + blobA + `","type":"bogus","offset":0,"length":1}]}]}`},
		{name: "short id", json: `{"packs":[{"id":"abcd"}]}`},
		{name: "non-hex id", json: `{"packs":[{"id":"zz` + packA[2:] + `"}]}`},
		{name: "trailing garbage", json: `{"packs":[]} x`},
		{name: "truncated", json: `{"packs":[{"id":"` + packA + `"`},
		{name: "empty", json: ``},
		{name: "top level null", json: `null`},
		{name: "top level array", json: `[]`},
		{name: "leading zero", json: `{"packs":[{"id":"` + packA + `","blobs":[{"id":"` + blobA + `","type":"data","offset":00,"length":1}]}]}`},
		{name: "control character in string", json: "{\"pa\tcks\":[]}"},
	} {
		t.Run(test.name, func(t *testing.T) {
			fast, fastOK, reference := decodeBoth(t, []byte(test.json))

			if test.mustParse {
				rtest.Assert(t, fastOK, "fast parser bounced input it should handle: %s", test.json)
			}
			if fastOK {
				rtest.Equals(t, reference, fast)
			}
		})
	}
}

// TestDecodeIndexFastRejectsDeepNesting checks that an unknown field cannot
// drive the skip path into unbounded recursion.
func TestDecodeIndexFastRejectsDeepNesting(t *testing.T) {
	deep := `{"x":` + strings.Repeat("[", 5000) + strings.Repeat("]", 5000) + `,"packs":[]}`

	_, ok := decodeIndexFast([]byte(deep))
	rtest.Assert(t, !ok, "fast parser accepted deeply nested input instead of bouncing")

	// DecodeIndex must still answer, via encoding/json.
	_, err := DecodeIndex([]byte(deep), restic.NewRandomID())
	rtest.OK(t, err)
}

// TestDecodeIndexSetsIDAndFinal pins the bookkeeping DecodeIndex does around
// whichever parser ran.
func TestDecodeIndexSetsIDAndFinal(t *testing.T) {
	id := restic.NewRandomID()

	for _, buf := range [][]byte{jsonExampleV2, []byte(`{"packs":[],"x":"A"}`)} {
		idx, err := DecodeIndex(buf, id)
		rtest.OK(t, err)
		rtest.Assert(t, idx.Final(), "decoded index is not final")
		ids, err := idx.IDs()
		rtest.OK(t, err)
		rtest.Equals(t, restic.IDs{id}, ids)
	}
}

// FuzzDecodeIndex is the real correctness argument for the hand-written
// parser: on arbitrary input it must either bounce or agree with
// encoding/json, byte for byte.
func FuzzDecodeIndex(f *testing.F) {
	f.Add(jsonExampleV1)
	f.Add(jsonExampleV2)
	f.Add([]byte(`{"packs":[{"id":"73d04e6125cf3c28a299cc2f3cca3b78ceac396e4fcf9575e34536b26782413c","blobs":[{"id":"3ec79977ef0cf5de7b08cd12b874cd0f62bbaf7f07f3497a5b1bbcc8cb39b1ce","type":"data","offset":0,"length":38,"uncompressed_length":9}]}]}`))
	f.Add([]byte(`{}`))
	f.Add([]byte(`{"packs":null}`))
	f.Add([]byte(`{"supersedes":[],"packs":[]}`))

	f.Fuzz(func(t *testing.T, buf []byte) {
		idx, ok := decodeIndexFast(buf)
		if !ok {
			return
		}

		ref, err := decodeIndexJSON(buf, restic.ID{})
		if err != nil {
			t.Fatalf("fast parser accepted input that encoding/json rejects: %q: %v", buf, err)
		}
		rtest.Equals(t, dump(ref), dump(idx))
	})
}
