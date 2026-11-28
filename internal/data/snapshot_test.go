package data_test

import (
	"context"
	"testing"
	"time"

	"github.com/restic/restic/internal/data"
	"github.com/restic/restic/internal/repository"
	rtest "github.com/restic/restic/internal/test"
)

func TestNewSnapshot(t *testing.T) {
	paths := []string{"/home/foobar"}

	_, err := data.NewSnapshot(paths, nil, "foo", time.Now())
	rtest.OK(t, err)
}

func TestTagList(t *testing.T) {
	paths := []string{"/home/foobar"}

	for _, test := range []struct {
		snapshotTags []string
		filterTags   []string
		match        bool
	}{
		{nil, []string{""}, true},
		{[]string{"foo"}, []string{""}, false},
		{nil, []string{"foo"}, false},
		{[]string{"foo"}, []string{"foo"}, true},
		// the empty tag requires an untagged snapshot, so it can never be
		// combined with another tag, no matter in which order they are given
		{nil, []string{"", "foo"}, false},
		{nil, []string{"foo", ""}, false},
		{[]string{"foo"}, []string{"", "foo"}, false},
		{[]string{"foo"}, []string{"foo", ""}, false},
	} {
		sn, err := data.NewSnapshot(paths, test.snapshotTags, "foo", time.Now())
		rtest.OK(t, err)

		rtest.Assert(t, sn.HasTags(test.filterTags) == test.match,
			"expected %v for snapshot tags %v and filter %v", test.match, test.snapshotTags, test.filterTags)
	}
}

func TestLoadJSONUnpacked(t *testing.T) {
	repository.TestAllVersions(t, testLoadJSONUnpacked)
}

func testLoadJSONUnpacked(t *testing.T, version uint) {
	repo, _, _ := repository.TestRepositoryWithVersion(t, version)

	// archive a snapshot
	sn := data.Snapshot{}
	sn.Hostname = "foobar"
	sn.Username = "test!"

	id, err := data.SaveSnapshot(context.TODO(), repo, &sn)
	rtest.OK(t, err)

	// restore
	sn2, err := data.LoadSnapshot(context.TODO(), repo, id)
	rtest.OK(t, err)

	rtest.Equals(t, sn.Hostname, sn2.Hostname)
	rtest.Equals(t, sn.Username, sn2.Username)
}

func TestSnapshotStringQuotesFields(t *testing.T) {
	ordinary := data.Snapshot{Paths: []string{"/home"}, Username: "alice", Hostname: "box"}.String()
	rtest.Equals(t, "snapshot [nil] of [/home] at 0001-01-01 00:00:00 +0000 UTC by alice@box", ordinary)

	hostile := data.Snapshot{
		Paths:    []string{"/ok", "a\nb"},
		Username: "u\nv",
		Hostname: "host",
	}.String()
	rtest.Equals(t, `snapshot [nil] of [/ok "a\nb"] at 0001-01-01 00:00:00 +0000 UTC by "u\nv"@host`, hostile)
}
