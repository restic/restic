package restic

import (
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
	for i := 0; i < 100; i++ {
		h.Type = DataBlob
		_, _ = random.Read(h.ID[:])
		s.Insert(h)
	}

	r := regexp.MustCompile(
		`^{(?:<(?:data|tree)/[0-9a-f]{8}> ){10}\(90 more\)}$`)
	str := s.String()
	rtest.Assert(t, r.MatchString(str), "%q doesn't match pattern", str)
}
