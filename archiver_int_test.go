package restic

import (
	"os"
	"testing"

	"github.com/restic/restic/pipe"
)

var treeJobs = []string{
	"foo/baz/subdir",
	"foo/bar",
	"foo",
	"quu/foo/file1",
	"quu/foo/file2",
	"quu/foo/file3",
	"quu/foo",
	"quu/fooz",
	"quu",
	"yy/a",
	"yy/b",
	"yy",
}

var pipeJobs = []string{
	"foo/baz/subdir",
	"foo/baz/subdir2", // subdir2 added
	"foo/bar",
	"foo",
	"quu/foo/file1", // file2 removed
	"quu/foo/file3",
	"quu/foo",
	"quu",
	"quv/file1", // files added and removed
	"quv/file2",
	"quv",
	"zz/file1", // new files removed and added at the end
	"zz/file2",
	"zz",
}

var resultJobs = []struct {
	path   string
	hasOld bool
}{
	{"foo/baz/subdir", true},
	{"foo/baz/subdir2", false},
	{"foo/bar", true},
	{"foo", true},
	{"quu/foo/file1", true},
	{"quu/foo/file3", true},
	{"quu/foo", true},
	{"quu", true},
	{"quv/file1", false},
	{"quv/file2", false},
	{"quv", false},
	{"zz/file1", false},
	{"zz/file2", false},
	{"zz", false},
}

type testPipeJob struct {
	path string
	err  error
	fi   os.FileInfo
	res  chan<- pipe.Result
}

func (j testPipeJob) Path() string               { return j.path }
func (j testPipeJob) Fullpath() string           { return j.path }
func (j testPipeJob) Error() error               { return j.err }
func (j testPipeJob) Info() os.FileInfo          { return j.fi }
func (j testPipeJob) Result() chan<- pipe.Result { return j.res }

func testTreeWalker(done <-chan struct{}, out chan<- WalkTreeJob) {
	for _, e := range treeJobs {
		select {
		case <-done:
			return
		case out <- WalkTreeJob{Path: e}:
		}
	}

	close(out)
}

func testPipeWalker(done <-chan struct{}, out chan<- pipe.Job) {
	for _, e := range pipeJobs {
		select {
		case <-done:
			return
		case out <- testPipeJob{path: e}:
		}
	}

	close(out)
}

func TestArchivePipe(t *testing.T) {
	done := make(chan struct{})

	treeCh := make(chan WalkTreeJob)
	pipeCh := make(chan pipe.Job)

	go testTreeWalker(done, treeCh)
	go testPipeWalker(done, pipeCh)

	p := ArchivePipe{Old: treeCh, New: pipeCh}

	ch := make(chan pipe.Job)

	go p.compare(done, ch)

	i := 0
	for job := range ch {
		if job.Path() != resultJobs[i].path {
			t.Fatalf("wrong job received: wanted %v, got %v", resultJobs[i], job)
		}
		i++
	}
}
