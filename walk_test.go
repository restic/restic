package restic_test

import (
	"flag"
	"path/filepath"
	"testing"

	"github.com/restic/restic"
	"github.com/restic/restic/pipe"
	. "github.com/restic/restic/test"
)

var testWalkDirectory = flag.String("test.walkdir", ".", "test walking a directory (globbing pattern, default: .)")

func TestWalkTree(t *testing.T) {
	dirs, err := filepath.Glob(*testWalkDirectory)
	OK(t, err)

	server := setupBackend(t)
	defer teardownBackend(t, server)
	key := setupKey(t, server, "geheim")
	server.SetKey(key)

	// archive a few files
	arch, err := restic.NewArchiver(server)
	OK(t, err)
	sn, _, err := arch.Snapshot(nil, dirs, nil)
	OK(t, err)

	// start benchmark
	// t.ResetTimer()

	// for i := 0; i < t.N; i++ {

	done := make(chan struct{})

	// start tree walker
	treeJobs := make(chan restic.WalkTreeJob)
	go restic.WalkTree(server, sn.Tree, done, treeJobs)

	// start filesystem walker
	fsJobs := make(chan pipe.Job)
	resCh := make(chan pipe.Result, 1)
	go pipe.Walk(dirs, done, fsJobs, resCh)

	for {
		// receive fs job
		fsJob, fsChOpen := <-fsJobs
		Assert(t, !fsChOpen || fsJob != nil,
			"received nil job from filesystem: %v %v", fsJob, fsChOpen)

		var path string
		fsEntries := 1
		switch j := fsJob.(type) {
		case pipe.Dir:
			path = j.Path()
			fsEntries = len(j.Entries)
		case pipe.Entry:
			path = j.Path()
		}

		// receive tree job
		treeJob, treeChOpen := <-treeJobs
		treeEntries := 1

		if treeJob.Tree != nil {
			treeEntries = len(treeJob.Tree.Nodes)
		}

		Assert(t, fsChOpen == treeChOpen,
			"one channel closed too early: fsChOpen %v, treeChOpen %v",
			fsChOpen, treeChOpen)

		if !fsChOpen || !treeChOpen {
			break
		}

		Assert(t, filepath.Base(path) == filepath.Base(treeJob.Path),
			"paths do not match: %q != %q", filepath.Base(path), filepath.Base(treeJob.Path))

		Assert(t, fsEntries == treeEntries,
			"wrong number of entries: %v != %v", fsEntries, treeEntries)
	}
	// }
}
