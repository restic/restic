package restic_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/restic/restic"
	"github.com/restic/restic/pipe"
	. "github.com/restic/restic/test"
)

func TestWalkTree(t *testing.T) {
	repo := SetupRepo()
	defer TeardownRepo(repo)

	dirs, err := filepath.Glob(TestWalkerPath)
	OK(t, err)

	// archive a few files
	arch := restic.NewArchiver(repo)
	sn, _, err := arch.Snapshot(nil, dirs, nil)
	OK(t, err)

	// flush repo, write all packs
	OK(t, repo.Flush())

	done := make(chan struct{})

	// start tree walker
	treeJobs := make(chan restic.WalkTreeJob)
	go restic.WalkTree(repo, *sn.Tree, done, treeJobs)

	// start filesystem walker
	fsJobs := make(chan pipe.Job)
	resCh := make(chan pipe.Result, 1)

	f := func(string, os.FileInfo) bool {
		return true
	}
	go pipe.Walk(dirs, f, done, fsJobs, resCh)

	for {
		// receive fs job
		fsJob, fsChOpen := <-fsJobs
		Assert(t, !fsChOpen || fsJob != nil,
			"received nil job from filesystem: %v %v", fsJob, fsChOpen)
		if fsJob != nil {
			OK(t, fsJob.Error())
		}

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

		OK(t, treeJob.Error)

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
}
