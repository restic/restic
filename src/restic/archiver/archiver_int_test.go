package archiver

import (
	"os"
	"testing"

	"restic/pipe"
	"restic/walk"
)

var treeJobs = []string{
	"foo/baz/subdir",
	"foo/baz",
	"foo",
	"quu/bar/file1",
	"quu/bar/file2",
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
	"foo/baz",
	"foo",
	"quu/bar/.file1.swp", // file with . added
	"quu/bar/file1",
	"quu/bar/file2",
	"quu/foo/file1", // file2 removed
	"quu/foo/file3",
	"quu/foo",
	"quu",
	"quv/file1", // files added and removed
	"quv/file2",
	"quv",
	"yy",
	"zz/file1", // files removed and added at the end
	"zz/file2",
	"zz",
}

var resultJobs = []struct {
	path   string
	action string
}{
	{"foo/baz/subdir", "same, not a file"},
	{"foo/baz/subdir2", "new, no old job"},
	{"foo/baz", "same, not a file"},
	{"foo", "same, not a file"},
	{"quu/bar/.file1.swp", "new, no old job"},
	{"quu/bar/file1", "same, not a file"},
	{"quu/bar/file2", "same, not a file"},
	{"quu/foo/file1", "same, not a file"},
	{"quu/foo/file3", "same, not a file"},
	{"quu/foo", "same, not a file"},
	{"quu", "same, not a file"},
	{"quv/file1", "new, no old job"},
	{"quv/file2", "new, no old job"},
	{"quv", "new, no old job"},
	{"yy", "same, not a file"},
	{"zz/file1", "testPipeJob"},
	{"zz/file2", "testPipeJob"},
	{"zz", "testPipeJob"},
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

func testTreeWalker(done <-chan struct{}, out chan<- walk.TreeJob) {
	for _, e := range treeJobs {
		select {
		case <-done:
			return
		case out <- walk.TreeJob{Path: e}:
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

	treeCh := make(chan walk.TreeJob)
	pipeCh := make(chan pipe.Job)

	go testTreeWalker(done, treeCh)
	go testPipeWalker(done, pipeCh)

	p := archivePipe{Old: treeCh, New: pipeCh}

	ch := make(chan pipe.Job)

	go p.compare(done, ch)

	i := 0
	for job := range ch {
		if job.Path() != resultJobs[i].path {
			t.Fatalf("wrong job received: wanted %v, got %v", resultJobs[i], job)
		}

		// switch j := job.(type) {
		// case archivePipeJob:
		// 	if j.action != resultJobs[i].action {
		// 		t.Fatalf("wrong action for %v detected: wanted %q, got %q", job.Path(), resultJobs[i].action, j.action)
		// 	}
		// case testPipeJob:
		// 	if resultJobs[i].action != "testPipeJob" {
		// 		t.Fatalf("unexpected testPipeJob, expected %q: %v", resultJobs[i].action, j)
		// 	}
		// }

		i++
	}
}
