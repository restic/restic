package pack

import (
	"testing"

	rtest "github.com/restic/restic/internal/test"
)

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

func TestBlobsSortNilSlice(t *testing.T) {
	var blobs Blobs
	blobs.Sort()
}
