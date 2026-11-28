//go:build go1.27

package index_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/restic/restic/internal/repository/index"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
)

// Test cases where the hand-rolled decoder is more strict than the basic encoding/json decoder.

func assertDecodeIndexErrorContains(t *testing.T, doc, msg string) {
	t.Helper()
	_, err := index.DecodeIndex([]byte(doc), restic.NewRandomID())
	if err == nil || !strings.Contains(err.Error(), msg) {
		t.Fatalf("expected error containing %q, got %v", msg, err)
	}
}

func TestDecodeIndexPackWithoutID(t *testing.T) {
	assertDecodeIndexErrorContains(t, `{"packs":[{"blobs":[]}]}`, "pack without ID found")
}

func TestDecodeIndexUint32Max(t *testing.T) {
	idx := mustDecodeIndex(t, `{"packs":[{"id":"73d04e6125cf3c28a299cc2f3cca3b78ceac396e4fcf9575e34536b26782413c","blobs":[{"id":"3ec79977ef0cf5de7b08cd12b874cd0f62bbaf7f07f3497a5b1bbcc8cb39b1ce","type":"data","offset":4294967295,"length":4294967295,"uncompressed_length":4294967295}]}]}`)

	list := idx.Lookup(restic.BlobHandle{
		ID:   restic.TestParseID("3ec79977ef0cf5de7b08cd12b874cd0f62bbaf7f07f3497a5b1bbcc8cb39b1ce"),
		Type: restic.DataBlob,
	}, nil)
	rtest.Equals(t, 1, len(list))
	rtest.Equals(t, uint(4294967295), list[0].Blob.Offset)
	rtest.Equals(t, uint(4294967295), list[0].Blob.Length)
	rtest.Equals(t, uint(4294967295), list[0].Blob.UncompressedLength)
}

func TestDecodeIndexRejectsUint32Overflow(t *testing.T) {
	// Same pack and blob in every case.
	const (
		pack = "73d04e6125cf3c28a299cc2f3cca3b78ceac396e4fcf9575e34536b26782413c"
		blob = "3ec79977ef0cf5de7b08cd12b874cd0f62bbaf7f07f3497a5b1bbcc8cb39b1ce"
	)
	tests := []struct {
		name string
		doc  string
	}{
		{
			"offset",
			fmt.Sprintf(`{"packs":[{"id":"%s","blobs":[{"id":"%s","type":"data","offset":4294967296,"length":1}]}]}`, pack, blob),
		},
		{
			"length",
			fmt.Sprintf(`{"packs":[{"id":"%s","blobs":[{"id":"%s","type":"data","offset":0,"length":4294967296}]}]}`, pack, blob),
		},
		{
			"uncompressed_length",
			fmt.Sprintf(`{"packs":[{"id":"%s","blobs":[{"id":"%s","type":"data","offset":0,"length":1,"uncompressed_length":4294967296}]}]}`, pack, blob),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertDecodeIndexErrorContains(t, test.doc, "invalid unsigned integer")
		})
	}
}

func TestDecodeIndexRejectsDuplicateFields(t *testing.T) {
	const (
		pack = "73d04e6125cf3c28a299cc2f3cca3b78ceac396e4fcf9575e34536b26782413c"
		blob = "3ec79977ef0cf5de7b08cd12b874cd0f62bbaf7f07f3497a5b1bbcc8cb39b1ce"
	)
	tests := []struct {
		name  string
		doc   string
		field string
	}{
		{
			"packs",
			fmt.Sprintf(`{"packs":[],"packs":[{"id":"%s","blobs":[]}]}`, pack),
			"packs",
		},
		{
			"pack id",
			fmt.Sprintf(`{"packs":[{"id":"%s","id":"%s","blobs":[]}]}`, pack, pack),
			"id",
		},
		{
			"blobs",
			fmt.Sprintf(`{"packs":[{"id":"%s","blobs":[],"blobs":[{"id":"%s","type":"data","offset":0,"length":1,"uncompressed_length":2}]}]}`, pack, blob),
			"blobs",
		},
		{
			"blob id",
			fmt.Sprintf(`{"packs":[{"id":"%s","blobs":[{"id":"%s","id":"%s","type":"data","offset":0,"length":1}]}]}`, pack, blob, blob),
			"id",
		},
		{
			"blob type",
			fmt.Sprintf(`{"packs":[{"id":"%s","blobs":[{"id":"%s","type":"data","type":"tree","offset":0,"length":1}]}]}`, pack, blob),
			"type",
		},
		{
			"offset",
			fmt.Sprintf(`{"packs":[{"id":"%s","blobs":[{"id":"%s","type":"data","offset":0,"offset":1,"length":1}]}]}`, pack, blob),
			"offset",
		},
		{
			"length",
			fmt.Sprintf(`{"packs":[{"id":"%s","blobs":[{"id":"%s","type":"data","offset":0,"length":1,"length":2}]}]}`, pack, blob),
			"length",
		},
		{
			"uncompressed_length",
			fmt.Sprintf(`{"packs":[{"id":"%s","blobs":[{"id":"%s","type":"data","offset":0,"length":1,"uncompressed_length":2,"uncompressed_length":3}]}]}`, pack, blob),
			"uncompressed_length",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertDecodeIndexErrorContains(t, test.doc, fmt.Sprintf("duplicate field %q", test.field))
		})
	}
}
