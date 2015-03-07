package restic_test

import (
	"flag"
	"path/filepath"
	"testing"

	"github.com/restic/restic"
	"github.com/restic/restic/pipe"
)

var testWalkDirectory = flag.String("test.walkdir", ".", "test walking a directory (globbing pattern, default: .)")

func TestWalkTree(t *testing.T) {
	dirs, err := filepath.Glob(*testWalkDirectory)
	ok(t, err)

	be := setupBackend(t)
	defer teardownBackend(t, be)
	key := setupKey(t, be, "geheim")
	server := restic.NewServerWithKey(be, key)

	// archive a few files
	arch, err := restic.NewArchiver(server)
	ok(t, err)
	sn, _, err := arch.Snapshot(nil, dirs, nil)
	ok(t, err)

	// start benchmark
	// t.ResetTimer()

	// for i := 0; i < t.N; i++ {

	done := make(chan struct{})

	// start tree walker
	treeJobs := make(chan restic.WalkTreeJob)
	go restic.WalkTree(server, sn.Tree.Storage, done, treeJobs)

	// start filesystem walker
	fsJobs := make(chan pipe.Job)
	resCh := make(chan pipe.Result, 1)
	go pipe.Walk(dirs, done, fsJobs, resCh)

	for {
		// receive fs job
		fsJob, fsChOpen := <-fsJobs
		assert(t, !fsChOpen || fsJob != nil,
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

		assert(t, fsChOpen == treeChOpen,
			"one channel closed too early: fsChOpen %v, treeChOpen %v",
			fsChOpen, treeChOpen)

		if !fsChOpen || !treeChOpen {
			break
		}

		assert(t, filepath.Base(path) == filepath.Base(treeJob.Path),
			"paths do not match: %q != %q", filepath.Base(path), filepath.Base(treeJob.Path))

		assert(t, fsEntries == treeEntries,
			"wrong number of entries: %v != %v", fsEntries, treeEntries)
	}
	// }
}
