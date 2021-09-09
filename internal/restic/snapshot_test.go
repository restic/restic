package restic_test

import (
	"testing"
	"time"

	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
)

func TestNewSnapshot(t *testing.T) {
	paths := []string{"/home/foobar"}

	_, err := restic.NewSnapshot(paths, nil, "foo", time.Now())
	rtest.OK(t, err)
}

func TestTagList(t *testing.T) {
	paths := []string{"/home/foobar"}
	tags := []string{""}

	sn, _ := restic.NewSnapshot(paths, nil, "foo", time.Now())

	r := sn.HasTags(tags)
	rtest.Assert(t, r, "Failed to match untagged snapshot")
}
