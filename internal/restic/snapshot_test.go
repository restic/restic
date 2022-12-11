package restic_test

import (
	"context"
	"testing"
	"time"

	"github.com/restic/restic/internal/repository"
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

func TestLoadJSONUnpacked(t *testing.T) {
	repository.TestAllVersions(t, testLoadJSONUnpacked)
}

func testLoadJSONUnpacked(t *testing.T, version uint) {
	repo := repository.TestRepositoryWithVersion(t, version)

	// archive a snapshot
	sn := restic.Snapshot{}
	sn.Hostname = "foobar"
	sn.Username = "test!"

	id, err := restic.SaveSnapshot(context.TODO(), repo, &sn)
	rtest.OK(t, err)

	// restore
	sn2, err := restic.LoadSnapshot(context.TODO(), repo, id)
	rtest.OK(t, err)

	rtest.Equals(t, sn.Hostname, sn2.Hostname)
	rtest.Equals(t, sn.Username, sn2.Username)
}
