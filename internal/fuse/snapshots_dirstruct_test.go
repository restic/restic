//go:build darwin || freebsd || linux
// +build darwin freebsd linux

package fuse

import (
	"strings"
	"testing"
	"time"

	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/test"
)

func TestPathsFromSn(t *testing.T) {
	id1, _ := restic.ParseID("1234567812345678123456781234567812345678123456781234567812345678")
	time1, _ := time.Parse("2006-01-02T15:04:05", "2021-01-01T00:00:01")
	sn1 := &restic.Snapshot{Hostname: "host", Username: "user", Tags: []string{"tag1", "tag2"}, Time: time1}
	restic.TestSetSnapshotID(t, sn1, id1)

	var p []string
	var s string

	p, s = pathsFromSn("ids/%i", "2006-01-02T15:04:05", sn1)
	test.Equals(t, []string{"ids/12345678"}, p)
	test.Equals(t, "", s)

	p, s = pathsFromSn("snapshots/%T", "2006-01-02T15:04:05", sn1)
	test.Equals(t, []string{"snapshots/"}, p)
	test.Equals(t, "2021-01-01T00:00:01", s)

	p, s = pathsFromSn("hosts/%h/%T", "2006-01-02T15:04:05", sn1)
	test.Equals(t, []string{"hosts/host/"}, p)
	test.Equals(t, "2021-01-01T00:00:01", s)

	p, s = pathsFromSn("tags/%t/%T", "2006-01-02T15:04:05", sn1)
	test.Equals(t, []string{"tags/tag1/", "tags/tag2/"}, p)
	test.Equals(t, "2021-01-01T00:00:01", s)

	p, s = pathsFromSn("users/%u/%T", "2006-01-02T15:04:05", sn1)
	test.Equals(t, []string{"users/user/"}, p)
	test.Equals(t, "2021-01-01T00:00:01", s)

	p, s = pathsFromSn("longids/%I", "2006-01-02T15:04:05", sn1)
	test.Equals(t, []string{"longids/1234567812345678123456781234567812345678123456781234567812345678"}, p)
	test.Equals(t, "", s)

	p, s = pathsFromSn("%T/%h", "2006/01/02", sn1)
	test.Equals(t, []string{"2021/01/01/host"}, p)
	test.Equals(t, "", s)

	p, s = pathsFromSn("%T/%i", "2006/01", sn1)
	test.Equals(t, []string{"2021/01/12345678"}, p)
	test.Equals(t, "", s)
}

func TestMakeDirs(t *testing.T) {
	pathTemplates := []string{"ids/%i", "snapshots/%T", "hosts/%h/%T",
		"tags/%t/%T", "users/%u/%T", "longids/%I", "%T/%h", "%T/%i",
	}
	timeTemplate := "2006/01/02"

	sds := &SnapshotsDirStructure{
		pathTemplates: pathTemplates,
		timeTemplate:  timeTemplate,
	}

	id0, _ := restic.ParseID("0000000012345678123456781234567812345678123456781234567812345678")
	time0, _ := time.Parse("2006-01-02T15:04:05", "2020-12-31T00:00:01")
	sn0 := &restic.Snapshot{Hostname: "host", Username: "user", Tags: []string{"tag1", "tag2"}, Time: time0}
	restic.TestSetSnapshotID(t, sn0, id0)

	id1, _ := restic.ParseID("1234567812345678123456781234567812345678123456781234567812345678")
	time1, _ := time.Parse("2006-01-02T15:04:05", "2021-01-01T00:00:01")
	sn1 := &restic.Snapshot{Hostname: "host", Username: "user", Tags: []string{"tag1", "tag2"}, Time: time1}
	restic.TestSetSnapshotID(t, sn1, id1)

	id2, _ := restic.ParseID("8765432112345678123456781234567812345678123456781234567812345678")
	time2, _ := time.Parse("2006-01-02T15:04:05", "2021-01-01T01:02:03")
	sn2 := &restic.Snapshot{Hostname: "host2", Username: "user2", Tags: []string{"tag2", "tag3", "tag4"}, Time: time2}
	restic.TestSetSnapshotID(t, sn2, id2)

	id3, _ := restic.ParseID("aaaaaaaa12345678123456781234567812345678123456781234567812345678")
	time3, _ := time.Parse("2006-01-02T15:04:05", "2021-01-01T01:02:03")
	sn3 := &restic.Snapshot{Hostname: "host", Username: "user2", Tags: []string{}, Time: time3}
	restic.TestSetSnapshotID(t, sn3, id3)

	sds.makeDirs(restic.Snapshots{sn0, sn1, sn2, sn3})

	expNames := make(map[string]*restic.Snapshot)
	expLatest := make(map[string]string)

	// entries for sn0
	expNames["/ids/00000000"] = sn0
	expNames["/snapshots/2020/12/31"] = sn0
	expNames["/hosts/host/2020/12/31"] = sn0
	expNames["/tags/tag1/2020/12/31"] = sn0
	expNames["/tags/tag2/2020/12/31"] = sn0
	expNames["/users/user/2020/12/31"] = sn0
	expNames["/longids/0000000012345678123456781234567812345678123456781234567812345678"] = sn0
	expNames["/2020/12/31/host"] = sn0
	expNames["/2020/12/31/00000000"] = sn0

	// entries for sn1
	expNames["/ids/12345678"] = sn1
	expNames["/snapshots/2021/01/01"] = sn1
	expNames["/hosts/host/2021/01/01"] = sn1
	expNames["/tags/tag1/2021/01/01"] = sn1
	expNames["/tags/tag2/2021/01/01"] = sn1
	expNames["/users/user/2021/01/01"] = sn1
	expNames["/longids/1234567812345678123456781234567812345678123456781234567812345678"] = sn1
	expNames["/2021/01/01/host"] = sn1
	expNames["/2021/01/01/12345678"] = sn1

	// entries for sn2
	expNames["/ids/87654321"] = sn2
	expNames["/snapshots/2021/01/01-1"] = sn2 // sn1 and sn2 have same time string
	expNames["/hosts/host2/2021/01/01"] = sn2
	expNames["/tags/tag2/2021/01/01-1"] = sn2 // sn1 and sn2 have same time string
	expNames["/tags/tag3/2021/01/01"] = sn2
	expNames["/tags/tag4/2021/01/01"] = sn2
	expNames["/users/user2/2021/01/01"] = sn2
	expNames["/longids/8765432112345678123456781234567812345678123456781234567812345678"] = sn2
	expNames["/2021/01/01/host2"] = sn2
	expNames["/2021/01/01/87654321"] = sn2

	// entries for sn3
	expNames["/ids/aaaaaaaa"] = sn3
	expNames["/snapshots/2021/01/01-2"] = sn3   // sn1 - sn3 have same time string
	expNames["/hosts/host/2021/01/01-1"] = sn3  // sn1 and sn3 have same time string
	expNames["/users/user2/2021/01/01-1"] = sn3 // sn2 and sn3 have same time string
	expNames["/longids/aaaaaaaa12345678123456781234567812345678123456781234567812345678"] = sn3
	expNames["/2021/01/01/host-1"] = sn3 // sn1 and sn3 have same time string and identical host
	expNames["/2021/01/01/aaaaaaaa"] = sn3

	// intermediate directories
	// sn0
	expNames["/ids"] = nil
	expNames[""] = nil
	expNames["/snapshots/2020/12"] = nil
	expNames["/snapshots/2020"] = nil
	expNames["/snapshots"] = nil
	expNames["/hosts/host/2020/12"] = nil
	expNames["/hosts/host/2020"] = nil
	expNames["/hosts/host"] = nil
	expNames["/hosts"] = nil
	expNames["/tags/tag1/2020/12"] = nil
	expNames["/tags/tag1/2020"] = nil
	expNames["/tags/tag1"] = nil
	expNames["/tags"] = nil
	expNames["/tags/tag2/2020/12"] = nil
	expNames["/tags/tag2/2020"] = nil
	expNames["/tags/tag2"] = nil
	expNames["/users/user/2020/12"] = nil
	expNames["/users/user/2020"] = nil
	expNames["/users/user"] = nil
	expNames["/users"] = nil
	expNames["/longids"] = nil
	expNames["/2020/12/31"] = nil
	expNames["/2020/12"] = nil
	expNames["/2020"] = nil

	// sn1
	expNames["/snapshots/2021/01"] = nil
	expNames["/snapshots/2021"] = nil
	expNames["/hosts/host/2021/01"] = nil
	expNames["/hosts/host/2021"] = nil
	expNames["/tags/tag1/2021/01"] = nil
	expNames["/tags/tag1/2021"] = nil
	expNames["/tags/tag2/2021/01"] = nil
	expNames["/tags/tag2/2021"] = nil
	expNames["/users/user/2021/01"] = nil
	expNames["/users/user/2021"] = nil
	expNames["/2021/01/01"] = nil
	expNames["/2021/01"] = nil
	expNames["/2021"] = nil

	// sn2
	expNames["/hosts/host2/2021/01"] = nil
	expNames["/hosts/host2/2021"] = nil
	expNames["/hosts/host2"] = nil
	expNames["/tags/tag3/2021/01"] = nil
	expNames["/tags/tag3/2021"] = nil
	expNames["/tags/tag3"] = nil
	expNames["/tags/tag4/2021/01"] = nil
	expNames["/tags/tag4/2021"] = nil
	expNames["/tags/tag4"] = nil
	expNames["/users/user2/2021/01"] = nil
	expNames["/users/user2/2021"] = nil
	expNames["/users/user2"] = nil

	// target snapshots for links
	expNames["/snapshots/latest"] = sn3 // sn1 - sn3 have same time string
	expNames["/hosts/host/latest"] = sn3
	expNames["/hosts/host2/latest"] = sn2
	expNames["/tags/tag1/latest"] = sn1
	expNames["/tags/tag2/latest"] = sn2 // sn1 and sn2 have same time string
	expNames["/tags/tag3/latest"] = sn2
	expNames["/tags/tag4/latest"] = sn2
	expNames["/users/user/latest"] = sn1
	expNames["/users/user2/latest"] = sn3 // sn2 and sn3 have same time string

	// latest links
	expLatest["/snapshots/latest"] = "2021/01/01-2" // sn1 - sn3 have same time string
	expLatest["/hosts/host/latest"] = "2021/01/01-1"
	expLatest["/hosts/host2/latest"] = "2021/01/01"
	expLatest["/tags/tag1/latest"] = "2021/01/01"
	expLatest["/tags/tag2/latest"] = "2021/01/01-1" // sn1 and sn2 have same time string
	expLatest["/tags/tag3/latest"] = "2021/01/01"
	expLatest["/tags/tag4/latest"] = "2021/01/01"
	expLatest["/users/user/latest"] = "2021/01/01"
	expLatest["/users/user2/latest"] = "2021/01/01-1" // sn2 and sn3 have same time string

	verifyEntries(t, expNames, expLatest, sds.entries)
}

func verifyEntries(t *testing.T, expNames map[string]*restic.Snapshot, expLatest map[string]string, entries map[string]*MetaDirData) {
	actNames := make(map[string]*restic.Snapshot)
	actLatest := make(map[string]string)
	for path, entry := range entries {
		actNames[path] = entry.snapshot
		if entry.linkTarget != "" {
			actLatest[path] = entry.linkTarget
		}
	}

	test.Equals(t, expNames, actNames)
	test.Equals(t, expLatest, actLatest)

	// verify tree integrity
	for path, entry := range entries {
		// check that all children are actually contained in entry.names
		for otherPath := range entries {
			if strings.HasPrefix(otherPath, path+"/") {
				sub := otherPath[len(path)+1:]
				// remaining path does not contain a directory
				test.Assert(t, strings.Contains(sub, "/") || (entry.names != nil && entry.names[sub] != nil), "missing entry %v in %v", sub, path)
			}
		}
		if entry.names == nil {
			continue
		}
		// child entries reference the correct MetaDirData
		for elem, subentry := range entry.names {
			test.Equals(t, entries[path+"/"+elem], subentry)
		}
	}
}

func TestMakeEmptyDirs(t *testing.T) {
	pathTemplates := []string{"ids/%i", "snapshots/%T", "hosts/%h/%T",
		"tags/%t/%T", "users/%u/%T", "longids/id-%I", "%T/%h", "%T/%i", "id-%i",
	}
	timeTemplate := "2006/01/02"

	sds := &SnapshotsDirStructure{
		pathTemplates: pathTemplates,
		timeTemplate:  timeTemplate,
	}
	sds.makeDirs(restic.Snapshots{})

	expNames := make(map[string]*restic.Snapshot)
	expLatest := make(map[string]string)

	// empty entries for dir structure
	expNames["/ids"] = nil
	expNames["/snapshots"] = nil
	expNames["/hosts"] = nil
	expNames["/tags"] = nil
	expNames["/users"] = nil
	expNames["/longids"] = nil
	expNames[""] = nil

	verifyEntries(t, expNames, expLatest, sds.entries)
}

func TestFilenameFromTag(t *testing.T) {
	for _, c := range []struct {
		tag, filename string
	}{
		{"", "_"},
		{".", "_"},
		{"..", "__"},
		{"%.", "%."},
		{"foo", "foo"},
		{"foo ", "foo "},
		{"foo/bar_baz", "foo_bar_baz"},
	} {
		test.Equals(t, c.filename, filenameFromTag(c.tag))
	}
}
