package restic

import (
	"fmt"
	"math/rand"
	"regexp"
	"testing"

	rtest "github.com/restic/restic/internal/test"
)

func TestBlobSetString(t *testing.T) {
	random := rand.New(rand.NewSource(42))

	s := NewBlobSet()

	rtest.Equals(t, "{}", s.String())

	id, _ := ParseID(
		"1111111111111111111111111111111111111111111111111111111111111111")
	s.Insert(BlobHandle{ID: id, Type: TreeBlob})
	rtest.Equals(t, "{<tree/11111111>}", s.String())

	var h BlobHandle
	for range 100 {
		h.Type = DataBlob
		_, _ = random.Read(h.ID[:])
		s.Insert(h)
	}

	r := regexp.MustCompile(
		`^{(?:<(?:data|tree)/[0-9a-f]{8}> ){10}\(91 more\)}$`)
	str := s.String()
	rtest.Assert(t, r.MatchString(str), "%q doesn't match pattern", str)
}

// makeBlobSet returns a set with n distinct data blob handles.
func makeBlobSet(n int) BlobSet {
	s := NewBlobSet()
	for i := range n {
		id, err := ParseID(fmt.Sprintf("%064x", i+1))
		if err != nil {
			panic(err)
		}
		s.Insert(BlobHandle{ID: id, Type: DataBlob})
	}
	return s
}

// TestBlobSetStringLimits checks the summary printed by BlobSet.String at and
// around the 10 element display limit: the "(N more)" count must equal the
// number of elements that were not listed.
func TestBlobSetStringLimits(t *testing.T) {
	for _, tt := range []struct {
		count int
		want  string
	}{
		{0, `^{}$`},
		{1, `^{<data/[0-9a-f]{8}>}$`},
		{9, `^{(?:<data/[0-9a-f]{8}> ){8}<data/[0-9a-f]{8}>}$`},  // below the limit: no summary
		{10, `^{(?:<data/[0-9a-f]{8}> ){9}<data/[0-9a-f]{8}>}$`}, // exactly at the limit: no summary
		{11, `^{(?:<data/[0-9a-f]{8}> ){10}\(1 more\)}$`},        // one above the limit
		{12, `^{(?:<data/[0-9a-f]{8}> ){10}\(2 more\)}$`},
		{101, `^{(?:<data/[0-9a-f]{8}> ){10}\(91 more\)}$`},
	} {
		t.Run(fmt.Sprintf("n=%d", tt.count), func(t *testing.T) {
			r := regexp.MustCompile(tt.want)
			str := makeBlobSet(tt.count).String()
			rtest.Assert(t, r.MatchString(str), "%q doesn't match pattern %q", str, tt.want)
		})
	}
}
