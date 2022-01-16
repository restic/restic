package restic_test

import (
	"path/filepath"
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

func TestSnapshotMatch(t *testing.T) {
	s := string(filepath.Separator)
	pathFooBar1 := []string{"home" + s + "foo" + s + "bar1"}
	pathFooBar2 := []string{"home" + s + "foo" + s + "bar2"}
	pathFooBars := []string{"home" + s + "foo" + s + "bar1", "home" + s + "foo" + s + "bar2"}
	pathFoo := []string{"home" + s + "foo", "home" + s + "xxx"}

	snFooBar1 := &restic.Snapshot{Paths: pathFooBar1}
	rtest.Equals(t, true, snFooBar1.MatchPaths(pathFooBar1))
	rtest.Equals(t, false, snFooBar1.MatchPaths(pathFooBar2))
	rtest.Equals(t, true, snFooBar1.MatchPaths(pathFooBars))
	rtest.Equals(t, true, snFooBar1.MatchPaths(pathFoo))

	snFoo := &restic.Snapshot{Paths: pathFoo}
	rtest.Equals(t, true, snFoo.MatchPaths(pathFooBar1))
	rtest.Equals(t, true, snFoo.MatchPaths(pathFooBar2))
	rtest.Equals(t, true, snFoo.MatchPaths(pathFooBars))
	rtest.Equals(t, true, snFoo.MatchPaths(pathFoo))

	snFooBars := &restic.Snapshot{Paths: pathFooBars}
	rtest.Equals(t, true, snFooBars.MatchPaths(pathFooBar1))
	rtest.Equals(t, true, snFooBars.MatchPaths(pathFooBar2))
	rtest.Equals(t, true, snFooBars.MatchPaths(pathFooBars))
	rtest.Equals(t, true, snFooBars.MatchPaths(pathFoo))

}

func TestSnapshotSupersedes(t *testing.T) {
	time1 := time.Now()
	time2 := time1.Add(time.Hour)

	s := string(filepath.Separator)
	pathFooBar1 := []string{"home" + s + "foo" + s + "bar1"}
	pathFooBar2 := []string{"home" + s + "foo" + s + "bar2"}
	pathFoo := []string{"home" + s + "foo"}
	pathFooBars := []string{"home" + s + "foo" + s + "bar1", "home" + s + "foo" + s + "bar2"}

	// test prior snapshot vs. later snapshot
	snTime1 := &restic.Snapshot{Paths: pathFooBar1, Time: time1}
	snTime2 := &restic.Snapshot{Paths: pathFooBar1, Time: time2}
	rtest.Equals(t, true, snTime2.Supersedes(snTime1, pathFooBar1))
	rtest.Equals(t, false, snTime1.Supersedes(snTime2, pathFooBar1))
	// equal times => supersedes
	rtest.Equals(t, true, snTime1.Supersedes(snTime1, pathFooBar1))

	// Note that for the path tests we here always use identical times.
	// In real-life scenarios additional to paths matching a snapshot
	// needs to newer than another snapshot to supersede.
	snFooBar1 := &restic.Snapshot{Paths: pathFooBar1, Time: time1}
	snFooBar2 := &restic.Snapshot{Paths: pathFooBar2, Time: time1}
	snFoo := &restic.Snapshot{Paths: pathFoo, Time: time1}
	snFooBars := &restic.Snapshot{Paths: pathFooBars, Time: time1}

	// only /foo/bar1 supersedes /foo/bar2 w.r.t. /foo/bar1
	rtest.Equals(t, true, snFooBar1.Supersedes(snFooBar2, pathFooBar1))
	rtest.Equals(t, false, snFooBar2.Supersedes(snFooBar1, pathFooBar1))
	// only /foo/bar2 supersedes /foo/bar1 w.r.t. /foo/bar2
	rtest.Equals(t, false, snFooBar1.Supersedes(snFooBar2, pathFooBar2))
	rtest.Equals(t, true, snFooBar2.Supersedes(snFooBar1, pathFooBar2))
	// neither /foo/bar1 nor /foo/bar2 supersede each other w.r.t. /foo
	rtest.Equals(t, false, snFooBar1.Supersedes(snFooBar2, pathFoo))
	rtest.Equals(t, false, snFooBar2.Supersedes(snFooBar1, pathFoo))
	// neither /foo/bar1 nor /foo/bar2 supersede each other w.r.t. [/foo/bar1,/foo/bar2]
	rtest.Equals(t, false, snFooBar1.Supersedes(snFooBar2, pathFooBars))
	rtest.Equals(t, false, snFooBar2.Supersedes(snFooBar1, pathFooBars))

	// /foo/bar1 and /foo supersede each other w.r.t. /foo/bar1
	rtest.Equals(t, true, snFooBar1.Supersedes(snFoo, pathFooBar1))
	rtest.Equals(t, true, snFoo.Supersedes(snFooBar1, pathFooBar1))
	// only /foo supersedes /foo/bar1 w.r.t. /foo/bar2
	rtest.Equals(t, false, snFooBar1.Supersedes(snFoo, pathFooBar2))
	rtest.Equals(t, true, snFoo.Supersedes(snFooBar1, pathFooBar2))
	// only /foo supersedes /foo/bar1 w.r.t. /foo
	rtest.Equals(t, false, snFooBar1.Supersedes(snFoo, pathFoo))
	rtest.Equals(t, true, snFoo.Supersedes(snFooBar1, pathFoo))
	// only /foo supersedes /foo/bar1 w.r.t. [/foo/bar1,/foo/bar2]
	rtest.Equals(t, false, snFooBar1.Supersedes(snFoo, pathFooBars))
	rtest.Equals(t, true, snFoo.Supersedes(snFooBar1, pathFooBars))

	// [/foo/bar1,/foo/bar2] and /foo supersede each other w.r.t. /foo/bar1
	rtest.Equals(t, true, snFooBars.Supersedes(snFoo, pathFooBar1))
	rtest.Equals(t, true, snFoo.Supersedes(snFooBars, pathFooBar1))
	// [/foo/bar1,/foo/bar2] and /foo supersede each other w.r.t. /foo/bar2
	rtest.Equals(t, true, snFooBars.Supersedes(snFoo, pathFooBar2))
	rtest.Equals(t, true, snFoo.Supersedes(snFooBars, pathFooBar2))
	// only /foo supersedes [/foo/bar1,/foo/bar2] w.r.t. /foo
	rtest.Equals(t, false, snFooBars.Supersedes(snFoo, pathFoo))
	rtest.Equals(t, true, snFoo.Supersedes(snFooBars, pathFoo))
	// [/foo/bar1,/foo/bar2] and /foo supersede each other w.r.t. [/foo/bar1,/foo/bar2]
	rtest.Equals(t, true, snFooBars.Supersedes(snFoo, pathFooBars))
	rtest.Equals(t, true, snFoo.Supersedes(snFooBars, pathFooBars))

	// /foo/bar1 and [/foo/bar1,/foo/bar2] supersede each other w.r.t. /foo/bar1
	rtest.Equals(t, true, snFooBar1.Supersedes(snFooBars, pathFooBar1))
	rtest.Equals(t, true, snFooBars.Supersedes(snFooBar1, pathFooBar1))
	// only [/foo/bar1,/foo/bar2] supersedes /foo/bar1 w.r.t. /foo/bar2
	rtest.Equals(t, false, snFooBar1.Supersedes(snFooBars, pathFooBar2))
	rtest.Equals(t, true, snFooBars.Supersedes(snFooBar1, pathFooBar2))
	// neither [/foo/bar1,/foo/bar2] nor /foo/bar1 supersede each other w.r.t. /foo
	rtest.Equals(t, false, snFooBar1.Supersedes(snFooBars, pathFoo))
	rtest.Equals(t, false, snFooBars.Supersedes(snFooBar1, pathFoo))
	// only [/foo/bar1,/foo/bar2] supersedes /foo/bar1 w.r.t. [/foo/bar1,/foo/bar2]
	rtest.Equals(t, false, snFooBar1.Supersedes(snFooBars, pathFooBars))
	rtest.Equals(t, true, snFooBars.Supersedes(snFooBar1, pathFooBars))
}
