package restic_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/restic/restic"
	"github.com/restic/restic/backend"
	"github.com/restic/restic/pack"
	"github.com/restic/restic/pipe"
	"github.com/restic/restic/repository"
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

type delayRepo struct {
	repo  *repository.Repository
	delay time.Duration
}

func (d delayRepo) LoadJSONPack(t pack.BlobType, id backend.ID, dst interface{}) error {
	time.Sleep(d.delay)
	return d.repo.LoadJSONPack(t, id, dst)
}

var repoFixture = filepath.Join("testdata", "walktree-test-repo.tar.gz")

func TestDelayedWalkTree(t *testing.T) {
	WithTestEnvironment(t, repoFixture, func(repodir string) {
		repo := OpenLocalRepo(t, repodir)
		OK(t, repo.LoadIndex())

		root, err := backend.ParseID("937a2f64f736c64ee700c6ab06f840c68c94799c288146a0e81e07f4c94254da")
		OK(t, err)

		dr := delayRepo{repo, 100 * time.Millisecond}

		// start tree walker
		treeJobs := make(chan restic.WalkTreeJob)
		go restic.WalkTree(dr, root, nil, treeJobs)

		for range treeJobs {
		}
	})
}

func BenchmarkDelayedWalkTree(t *testing.B) {
	WithTestEnvironment(t, repoFixture, func(repodir string) {
		repo := OpenLocalRepo(t, repodir)
		OK(t, repo.LoadIndex())

		root, err := backend.ParseID("937a2f64f736c64ee700c6ab06f840c68c94799c288146a0e81e07f4c94254da")
		OK(t, err)

		dr := delayRepo{repo, 10 * time.Millisecond}

		t.ResetTimer()

		for i := 0; i < t.N; i++ {
			// start tree walker
			treeJobs := make(chan restic.WalkTreeJob)
			go restic.WalkTree(dr, root, nil, treeJobs)

			for range treeJobs {
				// fmt.Printf("job: %v\n", job)
			}
		}
	})
}
