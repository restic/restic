package archiver

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/restic/restic/internal/checker"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/fs"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
	restictest "github.com/restic/restic/internal/test"
	tomb "gopkg.in/tomb.v2"
)

func prepareTempdirRepoSrc(t testing.TB, src TestDir) (tempdir string, repo restic.Repository, cleanup func()) {
	tempdir, removeTempdir := restictest.TempDir(t)
	repo, removeRepository := repository.TestRepository(t)

	TestCreateFiles(t, tempdir, src)

	cleanup = func() {
		removeRepository()
		removeTempdir()
	}

	return tempdir, repo, cleanup
}

func saveFile(t testing.TB, repo restic.Repository, filename string, filesystem fs.FS) (*restic.Node, ItemStats) {
	var tmb tomb.Tomb
	ctx := tmb.Context(context.Background())

	arch := New(repo, filesystem, Options{})
	arch.runWorkers(ctx, &tmb)

	arch.Error = func(item string, fi os.FileInfo, err error) error {
		t.Errorf("archiver error for %v: %v", item, err)
		return err
	}

	var (
		completeCallbackNode  *restic.Node
		completeCallbackStats ItemStats
		completeCallback      bool

		startCallback bool
	)

	complete := func(node *restic.Node, stats ItemStats) {
		completeCallback = true
		completeCallbackNode = node
		completeCallbackStats = stats
	}

	start := func() {
		startCallback = true
	}

	file, err := arch.FS.OpenFile(filename, fs.O_RDONLY|fs.O_NOFOLLOW, 0)
	if err != nil {
		t.Fatal(err)
	}

	fi, err := file.Stat()
	if err != nil {
		t.Fatal(err)
	}

	res := arch.fileSaver.Save(ctx, "/", file, fi, start, complete)

	res.Wait(ctx)
	if res.Err() != nil {
		t.Fatal(res.Err())
	}

	tmb.Kill(nil)
	err = tmb.Wait()
	if err != nil {
		t.Fatal(err)
	}

	err = repo.Flush(ctx)
	if err != nil {
		t.Fatal(err)
	}

	err = repo.SaveIndex(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if !startCallback {
		t.Errorf("start callback did not happen")
	}

	if !completeCallback {
		t.Errorf("complete callback did not happen")
	}

	if completeCallbackNode == nil {
		t.Errorf("no node returned for complete callback")
	}

	if completeCallbackNode != nil && !res.Node().Equals(*completeCallbackNode) {
		t.Errorf("different node returned for complete callback")
	}

	if completeCallbackStats != res.Stats() {
		t.Errorf("different stats return for complete callback, want:\n  %v\ngot:\n  %v", res.Stats(), completeCallbackStats)
	}

	return res.Node(), res.Stats()
}

func TestArchiverSaveFile(t *testing.T) {
	var tests = []TestFile{
		TestFile{Content: ""},
		TestFile{Content: "foo"},
		TestFile{Content: string(restictest.Random(23, 12*1024*1024+1287898))},
	}

	for _, testfile := range tests {
		t.Run("", func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			tempdir, repo, cleanup := prepareTempdirRepoSrc(t, TestDir{"file": testfile})
			defer cleanup()

			node, stats := saveFile(t, repo, filepath.Join(tempdir, "file"), fs.Track{FS: fs.Local{}})

			TestEnsureFileContent(ctx, t, repo, "file", node, testfile)
			if stats.DataSize != uint64(len(testfile.Content)) {
				t.Errorf("wrong stats returned in DataSize, want %d, got %d", len(testfile.Content), stats.DataSize)
			}
			if stats.DataBlobs <= 0 && len(testfile.Content) > 0 {
				t.Errorf("wrong stats returned in DataBlobs, want > 0, got %d", stats.DataBlobs)
			}
			if stats.TreeSize != 0 {
				t.Errorf("wrong stats returned in TreeSize, want 0, got %d", stats.TreeSize)
			}
			if stats.TreeBlobs != 0 {
				t.Errorf("wrong stats returned in DataBlobs, want 0, got %d", stats.DataBlobs)
			}
		})
	}
}

func TestArchiverSaveFileReaderFS(t *testing.T) {
	var tests = []struct {
		Data string
	}{
		{Data: ""},
		{Data: "foo"},
		{Data: string(restictest.Random(23, 12*1024*1024+1287898))},
	}

	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			repo, cleanup := repository.TestRepository(t)
			defer cleanup()

			ts := time.Now()
			filename := "xx"
			readerFs := &fs.Reader{
				ModTime:    ts,
				Mode:       0123,
				Name:       filename,
				ReadCloser: ioutil.NopCloser(strings.NewReader(test.Data)),
			}

			node, stats := saveFile(t, repo, filename, readerFs)

			TestEnsureFileContent(ctx, t, repo, "file", node, TestFile{Content: test.Data})
			if stats.DataSize != uint64(len(test.Data)) {
				t.Errorf("wrong stats returned in DataSize, want %d, got %d", len(test.Data), stats.DataSize)
			}
			if stats.DataBlobs <= 0 && len(test.Data) > 0 {
				t.Errorf("wrong stats returned in DataBlobs, want > 0, got %d", stats.DataBlobs)
			}
			if stats.TreeSize != 0 {
				t.Errorf("wrong stats returned in TreeSize, want 0, got %d", stats.TreeSize)
			}
			if stats.TreeBlobs != 0 {
				t.Errorf("wrong stats returned in DataBlobs, want 0, got %d", stats.DataBlobs)
			}
		})
	}
}

func TestArchiverSave(t *testing.T) {
	var tests = []TestFile{
		TestFile{Content: ""},
		TestFile{Content: "foo"},
		TestFile{Content: string(restictest.Random(23, 12*1024*1024+1287898))},
	}

	for _, testfile := range tests {
		t.Run("", func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			tempdir, repo, cleanup := prepareTempdirRepoSrc(t, TestDir{"file": testfile})
			defer cleanup()

			var tmb tomb.Tomb

			arch := New(repo, fs.Track{FS: fs.Local{}}, Options{})
			arch.Error = func(item string, fi os.FileInfo, err error) error {
				t.Errorf("archiver error for %v: %v", item, err)
				return err
			}
			arch.runWorkers(tmb.Context(ctx), &tmb)

			node, excluded, err := arch.Save(ctx, "/", filepath.Join(tempdir, "file"), nil)
			if err != nil {
				t.Fatal(err)
			}

			if excluded {
				t.Errorf("Save() excluded the node, that's unexpected")
			}

			node.wait(ctx)
			if node.err != nil {
				t.Fatal(node.err)
			}

			if node.node == nil {
				t.Fatalf("returned node is nil")
			}

			stats := node.stats

			err = repo.Flush(ctx)
			if err != nil {
				t.Fatal(err)
			}

			TestEnsureFileContent(ctx, t, repo, "file", node.node, testfile)
			if stats.DataSize != uint64(len(testfile.Content)) {
				t.Errorf("wrong stats returned in DataSize, want %d, got %d", len(testfile.Content), stats.DataSize)
			}
			if stats.DataBlobs <= 0 && len(testfile.Content) > 0 {
				t.Errorf("wrong stats returned in DataBlobs, want > 0, got %d", stats.DataBlobs)
			}
			if stats.TreeSize != 0 {
				t.Errorf("wrong stats returned in TreeSize, want 0, got %d", stats.TreeSize)
			}
			if stats.TreeBlobs != 0 {
				t.Errorf("wrong stats returned in DataBlobs, want 0, got %d", stats.DataBlobs)
			}
		})
	}
}

func TestArchiverSaveReaderFS(t *testing.T) {
	var tests = []struct {
		Data string
	}{
		{Data: ""},
		{Data: "foo"},
		{Data: string(restictest.Random(23, 12*1024*1024+1287898))},
	}

	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			repo, cleanup := repository.TestRepository(t)
			defer cleanup()

			ts := time.Now()
			filename := "xx"
			readerFs := &fs.Reader{
				ModTime:    ts,
				Mode:       0123,
				Name:       filename,
				ReadCloser: ioutil.NopCloser(strings.NewReader(test.Data)),
			}

			var tmb tomb.Tomb

			arch := New(repo, readerFs, Options{})
			arch.Error = func(item string, fi os.FileInfo, err error) error {
				t.Errorf("archiver error for %v: %v", item, err)
				return err
			}
			arch.runWorkers(tmb.Context(ctx), &tmb)

			node, excluded, err := arch.Save(ctx, "/", filename, nil)
			t.Logf("Save returned %v %v", node, err)
			if err != nil {
				t.Fatal(err)
			}

			if excluded {
				t.Errorf("Save() excluded the node, that's unexpected")
			}

			node.wait(ctx)
			if node.err != nil {
				t.Fatal(node.err)
			}

			if node.node == nil {
				t.Fatalf("returned node is nil")
			}

			stats := node.stats

			err = repo.Flush(ctx)
			if err != nil {
				t.Fatal(err)
			}

			TestEnsureFileContent(ctx, t, repo, "file", node.node, TestFile{Content: test.Data})
			if stats.DataSize != uint64(len(test.Data)) {
				t.Errorf("wrong stats returned in DataSize, want %d, got %d", len(test.Data), stats.DataSize)
			}
			if stats.DataBlobs <= 0 && len(test.Data) > 0 {
				t.Errorf("wrong stats returned in DataBlobs, want > 0, got %d", stats.DataBlobs)
			}
			if stats.TreeSize != 0 {
				t.Errorf("wrong stats returned in TreeSize, want 0, got %d", stats.TreeSize)
			}
			if stats.TreeBlobs != 0 {
				t.Errorf("wrong stats returned in DataBlobs, want 0, got %d", stats.DataBlobs)
			}
		})
	}
}

func BenchmarkArchiverSaveFileSmall(b *testing.B) {
	const fileSize = 4 * 1024
	d := TestDir{"file": TestFile{
		Content: string(restictest.Random(23, fileSize)),
	}}

	b.SetBytes(fileSize)

	for i := 0; i < b.N; i++ {
		b.StopTimer()
		tempdir, repo, cleanup := prepareTempdirRepoSrc(b, d)
		b.StartTimer()

		_, stats := saveFile(b, repo, filepath.Join(tempdir, "file"), fs.Track{FS: fs.Local{}})

		b.StopTimer()
		if stats.DataSize != fileSize {
			b.Errorf("wrong stats returned in DataSize, want %d, got %d", fileSize, stats.DataSize)
		}
		if stats.DataBlobs <= 0 {
			b.Errorf("wrong stats returned in DataBlobs, want > 0, got %d", stats.DataBlobs)
		}
		if stats.TreeSize != 0 {
			b.Errorf("wrong stats returned in TreeSize, want 0, got %d", stats.TreeSize)
		}
		if stats.TreeBlobs != 0 {
			b.Errorf("wrong stats returned in DataBlobs, want 0, got %d", stats.DataBlobs)
		}
		cleanup()
		b.StartTimer()
	}
}

func BenchmarkArchiverSaveFileLarge(b *testing.B) {
	const fileSize = 40*1024*1024 + 1287898
	d := TestDir{"file": TestFile{
		Content: string(restictest.Random(23, fileSize)),
	}}

	b.SetBytes(fileSize)

	for i := 0; i < b.N; i++ {
		b.StopTimer()
		tempdir, repo, cleanup := prepareTempdirRepoSrc(b, d)
		b.StartTimer()

		_, stats := saveFile(b, repo, filepath.Join(tempdir, "file"), fs.Track{FS: fs.Local{}})

		b.StopTimer()
		if stats.DataSize != fileSize {
			b.Errorf("wrong stats returned in DataSize, want %d, got %d", fileSize, stats.DataSize)
		}
		if stats.DataBlobs <= 0 {
			b.Errorf("wrong stats returned in DataBlobs, want > 0, got %d", stats.DataBlobs)
		}
		if stats.TreeSize != 0 {
			b.Errorf("wrong stats returned in TreeSize, want 0, got %d", stats.TreeSize)
		}
		if stats.TreeBlobs != 0 {
			b.Errorf("wrong stats returned in DataBlobs, want 0, got %d", stats.DataBlobs)
		}
		cleanup()
		b.StartTimer()
	}
}

type blobCountingRepo struct {
	restic.Repository

	m     sync.Mutex
	saved map[restic.BlobHandle]uint
}

func (repo *blobCountingRepo) SaveBlob(ctx context.Context, t restic.BlobType, buf []byte, id restic.ID) (restic.ID, error) {
	id, err := repo.Repository.SaveBlob(ctx, t, buf, id)
	h := restic.BlobHandle{ID: id, Type: t}
	repo.m.Lock()
	repo.saved[h]++
	repo.m.Unlock()
	return id, err
}

func (repo *blobCountingRepo) SaveTree(ctx context.Context, t *restic.Tree) (restic.ID, error) {
	id, err := repo.Repository.SaveTree(ctx, t)
	h := restic.BlobHandle{ID: id, Type: restic.TreeBlob}
	repo.m.Lock()
	repo.saved[h]++
	repo.m.Unlock()
	return id, err
}

func appendToFile(t testing.TB, filename string, data []byte) {
	f, err := os.OpenFile(filename, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatal(err)
	}

	_, err = f.Write(data)
	if err != nil {
		_ = f.Close()
		t.Fatal(err)
	}

	err = f.Close()
	if err != nil {
		t.Fatal(err)
	}
}

func TestArchiverSaveFileIncremental(t *testing.T) {
	tempdir, removeTempdir := restictest.TempDir(t)
	defer removeTempdir()

	testRepo, removeRepository := repository.TestRepository(t)
	defer removeRepository()

	repo := &blobCountingRepo{
		Repository: testRepo,
		saved:      make(map[restic.BlobHandle]uint),
	}

	data := restictest.Random(23, 512*1024+887898)
	testfile := filepath.Join(tempdir, "testfile")

	for i := 0; i < 3; i++ {
		appendToFile(t, testfile, data)
		node, _ := saveFile(t, repo, testfile, fs.Track{FS: fs.Local{}})

		t.Logf("node blobs: %v", node.Content)

		for h, n := range repo.saved {
			if n > 1 {
				t.Errorf("iteration %v: blob %v saved more than once (%d times)", i, h, n)
			}
		}
	}
}

func save(t testing.TB, filename string, data []byte) {
	f, err := os.Create(filename)
	if err != nil {
		t.Fatal(err)
	}

	_, err = f.Write(data)
	if err != nil {
		t.Fatal(err)
	}

	err = f.Sync()
	if err != nil {
		t.Fatal(err)
	}

	err = f.Close()
	if err != nil {
		t.Fatal(err)
	}
}

func lstat(t testing.TB, name string) os.FileInfo {
	fi, err := os.Lstat(name)
	if err != nil {
		t.Fatal(err)
	}

	return fi
}

func setTimestamp(t testing.TB, filename string, atime, mtime time.Time) {
	var utimes = [...]syscall.Timespec{
		syscall.NsecToTimespec(atime.UnixNano()),
		syscall.NsecToTimespec(mtime.UnixNano()),
	}

	err := syscall.UtimesNano(filename, utimes[:])
	if err != nil {
		t.Fatal(err)
	}
}

func remove(t testing.TB, filename string) {
	err := os.Remove(filename)
	if err != nil {
		t.Fatal(err)
	}
}

func nodeFromFI(t testing.TB, filename string, fi os.FileInfo) *restic.Node {
	node, err := restic.NodeFromFileInfo(filename, fi)
	if err != nil {
		t.Fatal(err)
	}

	return node
}

func TestFileChanged(t *testing.T) {
	var defaultContent = []byte("foobar")

	var d = 50 * time.Millisecond
	if runtime.GOOS == "darwin" {
		// on older darwin instances the file system only supports one second
		// granularity
		d = time.Second
	}

	sleep := func() {
		time.Sleep(d)
	}

	var tests = []struct {
		Name    string
		Content []byte
		Modify  func(t testing.TB, filename string)
	}{
		{
			Name: "same-content-new-file",
			Modify: func(t testing.TB, filename string) {
				remove(t, filename)
				sleep()
				save(t, filename, defaultContent)
			},
		},
		{
			Name: "same-content-new-timestamp",
			Modify: func(t testing.TB, filename string) {
				sleep()
				save(t, filename, defaultContent)
			},
		},
		{
			Name: "other-content",
			Modify: func(t testing.TB, filename string) {
				remove(t, filename)
				sleep()
				save(t, filename, []byte("xxxxxx"))
			},
		},
		{
			Name: "longer-content",
			Modify: func(t testing.TB, filename string) {
				save(t, filename, []byte("xxxxxxxxxxxxxxxxxxxxxx"))
			},
		},
		{
			Name: "new-file",
			Modify: func(t testing.TB, filename string) {
				remove(t, filename)
				sleep()
				save(t, filename, defaultContent)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			tempdir, cleanup := restictest.TempDir(t)
			defer cleanup()

			filename := filepath.Join(tempdir, "file")
			content := defaultContent
			if test.Content != nil {
				content = test.Content
			}
			save(t, filename, content)

			fiBefore := lstat(t, filename)
			node := nodeFromFI(t, filename, fiBefore)

			if fileChanged(fiBefore, node) {
				t.Fatalf("unchanged file detected as changed")
			}

			test.Modify(t, filename)

			fiAfter := lstat(t, filename)
			if !fileChanged(fiAfter, node) {
				t.Fatalf("modified file detected as unchanged")
			}
		})
	}
}

func TestFilChangedSpecialCases(t *testing.T) {
	tempdir, cleanup := restictest.TempDir(t)
	defer cleanup()

	filename := filepath.Join(tempdir, "file")
	content := []byte("foobar")
	save(t, filename, content)

	t.Run("nil-node", func(t *testing.T) {
		fi := lstat(t, filename)
		if !fileChanged(fi, nil) {
			t.Fatal("nil node detected as unchanged")
		}
	})

	t.Run("type-change", func(t *testing.T) {
		fi := lstat(t, filename)
		node := nodeFromFI(t, filename, fi)
		node.Type = "symlink"
		if !fileChanged(fi, node) {
			t.Fatal("node with changed type detected as unchanged")
		}
	})
}

func TestArchiverSaveDir(t *testing.T) {
	const targetNodeName = "targetdir"

	var tests = []struct {
		src    TestDir
		chdir  string
		target string
		want   TestDir
	}{
		{
			src: TestDir{
				"targetfile": TestFile{Content: string(restictest.Random(888, 2*1024*1024+5000))},
			},
			target: ".",
			want: TestDir{
				"targetdir": TestDir{
					"targetfile": TestFile{Content: string(restictest.Random(888, 2*1024*1024+5000))},
				},
			},
		},
		{
			src: TestDir{
				"targetdir": TestDir{
					"foo":        TestFile{Content: "foo"},
					"emptyfile":  TestFile{Content: ""},
					"bar":        TestFile{Content: "XXXXXXXXXXXXXXXXXXXXXXXXXXXXXX"},
					"largefile":  TestFile{Content: string(restictest.Random(888, 2*1024*1024+5000))},
					"largerfile": TestFile{Content: string(restictest.Random(234, 5*1024*1024+5000))},
				},
			},
			target: "targetdir",
		},
		{
			src: TestDir{
				"foo":       TestFile{Content: "foo"},
				"emptyfile": TestFile{Content: ""},
				"bar":       TestFile{Content: "XXXXXXXXXXXXXXXXXXXXXXXXXXXXXX"},
			},
			target: ".",
			want: TestDir{
				"targetdir": TestDir{
					"foo":       TestFile{Content: "foo"},
					"emptyfile": TestFile{Content: ""},
					"bar":       TestFile{Content: "XXXXXXXXXXXXXXXXXXXXXXXXXXXXXX"},
				},
			},
		},
		{
			src: TestDir{
				"foo": TestDir{
					"subdir": TestDir{
						"x": TestFile{Content: "xxx"},
						"y": TestFile{Content: "yyyyyyyyyyyyyyyy"},
						"z": TestFile{Content: "zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz"},
					},
					"file": TestFile{Content: "just a test"},
				},
			},
			chdir:  "foo/subdir",
			target: "../../",
			want: TestDir{
				"targetdir": TestDir{
					"foo": TestDir{
						"subdir": TestDir{
							"x": TestFile{Content: "xxx"},
							"y": TestFile{Content: "yyyyyyyyyyyyyyyy"},
							"z": TestFile{Content: "zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz"},
						},
						"file": TestFile{Content: "just a test"},
					},
				},
			},
		},
		{
			src: TestDir{
				"foo": TestDir{
					"file":  TestFile{Content: "just a test"},
					"file2": TestFile{Content: "again"},
				},
			},
			target: "./foo",
			want: TestDir{
				"targetdir": TestDir{
					"file":  TestFile{Content: "just a test"},
					"file2": TestFile{Content: "again"},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			var tmb tomb.Tomb
			ctx := tmb.Context(context.Background())

			tempdir, repo, cleanup := prepareTempdirRepoSrc(t, test.src)
			defer cleanup()

			arch := New(repo, fs.Track{FS: fs.Local{}}, Options{})
			arch.runWorkers(ctx, &tmb)

			chdir := tempdir
			if test.chdir != "" {
				chdir = filepath.Join(chdir, test.chdir)
			}

			back := fs.TestChdir(t, chdir)
			defer back()

			fi, err := fs.Lstat(test.target)
			if err != nil {
				t.Fatal(err)
			}

			ft, err := arch.SaveDir(ctx, "/", fi, test.target, nil)
			if err != nil {
				t.Fatal(err)
			}

			ft.Wait(ctx)
			node, stats := ft.Node(), ft.Stats()

			tmb.Kill(nil)
			err = tmb.Wait()
			if err != nil {
				t.Fatal(err)
			}

			t.Logf("stats: %v", stats)
			if stats.DataSize != 0 {
				t.Errorf("wrong stats returned in DataSize, want 0, got %d", stats.DataSize)
			}
			if stats.DataBlobs != 0 {
				t.Errorf("wrong stats returned in DataBlobs, want 0, got %d", stats.DataBlobs)
			}
			if stats.TreeSize <= 0 {
				t.Errorf("wrong stats returned in TreeSize, want > 0, got %d", stats.TreeSize)
			}
			if stats.TreeBlobs <= 0 {
				t.Errorf("wrong stats returned in TreeBlobs, want > 0, got %d", stats.TreeBlobs)
			}

			node.Name = targetNodeName
			tree := &restic.Tree{Nodes: []*restic.Node{node}}
			treeID, err := repo.SaveTree(ctx, tree)
			if err != nil {
				t.Fatal(err)
			}

			err = repo.Flush(ctx)
			if err != nil {
				t.Fatal(err)
			}

			err = repo.SaveIndex(ctx)
			if err != nil {
				t.Fatal(err)
			}

			want := test.want
			if want == nil {
				want = test.src
			}
			TestEnsureTree(ctx, t, "/", repo, treeID, want)
		})
	}
}

func TestArchiverSaveDirIncremental(t *testing.T) {
	tempdir, removeTempdir := restictest.TempDir(t)
	defer removeTempdir()

	testRepo, removeRepository := repository.TestRepository(t)
	defer removeRepository()

	repo := &blobCountingRepo{
		Repository: testRepo,
		saved:      make(map[restic.BlobHandle]uint),
	}

	appendToFile(t, filepath.Join(tempdir, "testfile"), []byte("foobar"))

	// save the empty directory several times in a row, then have a look if the
	// archiver did save the same tree several times
	for i := 0; i < 5; i++ {
		var tmb tomb.Tomb
		ctx := tmb.Context(context.Background())

		arch := New(repo, fs.Track{FS: fs.Local{}}, Options{})
		arch.runWorkers(ctx, &tmb)

		fi, err := fs.Lstat(tempdir)
		if err != nil {
			t.Fatal(err)
		}

		ft, err := arch.SaveDir(ctx, "/", fi, tempdir, nil)
		if err != nil {
			t.Fatal(err)
		}

		ft.Wait(ctx)
		node, stats := ft.Node(), ft.Stats()

		tmb.Kill(nil)
		err = tmb.Wait()
		if err != nil {
			t.Fatal(err)
		}

		if i == 0 {
			// operation must have added new tree data
			if stats.DataSize != 0 {
				t.Errorf("wrong stats returned in DataSize, want 0, got %d", stats.DataSize)
			}
			if stats.DataBlobs != 0 {
				t.Errorf("wrong stats returned in DataBlobs, want 0, got %d", stats.DataBlobs)
			}
			if stats.TreeSize <= 0 {
				t.Errorf("wrong stats returned in TreeSize, want > 0, got %d", stats.TreeSize)
			}
			if stats.TreeBlobs <= 0 {
				t.Errorf("wrong stats returned in TreeBlobs, want > 0, got %d", stats.TreeBlobs)
			}
		} else {
			// operation must not have added any new data
			if stats.DataSize != 0 {
				t.Errorf("wrong stats returned in DataSize, want 0, got %d", stats.DataSize)
			}
			if stats.DataBlobs != 0 {
				t.Errorf("wrong stats returned in DataBlobs, want 0, got %d", stats.DataBlobs)
			}
			if stats.TreeSize != 0 {
				t.Errorf("wrong stats returned in TreeSize, want 0, got %d", stats.TreeSize)
			}
			if stats.TreeBlobs != 0 {
				t.Errorf("wrong stats returned in TreeBlobs, want 0, got %d", stats.TreeBlobs)
			}
		}

		t.Logf("node subtree %v", node.Subtree)

		err = repo.Flush(ctx)
		if err != nil {
			t.Fatal(err)
		}

		err = repo.SaveIndex(ctx)
		if err != nil {
			t.Fatal(err)
		}

		for h, n := range repo.saved {
			if n > 1 {
				t.Errorf("iteration %v: blob %v saved more than once (%d times)", i, h, n)
			}
		}
	}
}

func TestArchiverSaveTree(t *testing.T) {
	symlink := func(from, to string) func(t testing.TB) {
		return func(t testing.TB) {
			err := os.Symlink(from, to)
			if err != nil {
				t.Fatal(err)
			}
		}
	}

	var tests = []struct {
		src     TestDir
		prepare func(t testing.TB)
		targets []string
		want    TestDir
	}{
		{
			src: TestDir{
				"targetfile": TestFile{Content: string("foobar")},
			},
			targets: []string{"targetfile"},
			want: TestDir{
				"targetfile": TestFile{Content: string("foobar")},
			},
		},
		{
			src: TestDir{
				"targetfile": TestFile{Content: string("foobar")},
			},
			prepare: symlink("targetfile", "filesymlink"),
			targets: []string{"targetfile", "filesymlink"},
			want: TestDir{
				"targetfile":  TestFile{Content: string("foobar")},
				"filesymlink": TestSymlink{Target: "targetfile"},
			},
		},
		{
			src: TestDir{
				"dir": TestDir{
					"subdir": TestDir{
						"subsubdir": TestDir{
							"targetfile": TestFile{Content: string("foobar")},
						},
					},
					"otherfile": TestFile{Content: string("xxx")},
				},
			},
			prepare: symlink("subdir", filepath.FromSlash("dir/symlink")),
			targets: []string{filepath.FromSlash("dir/symlink")},
			want: TestDir{
				"dir": TestDir{
					"symlink": TestSymlink{Target: "subdir"},
				},
			},
		},
		{
			src: TestDir{
				"dir": TestDir{
					"subdir": TestDir{
						"subsubdir": TestDir{
							"targetfile": TestFile{Content: string("foobar")},
						},
					},
					"otherfile": TestFile{Content: string("xxx")},
				},
			},
			prepare: symlink("subdir", filepath.FromSlash("dir/symlink")),
			targets: []string{filepath.FromSlash("dir/symlink/subsubdir")},
			want: TestDir{
				"dir": TestDir{
					"symlink": TestDir{
						"subsubdir": TestDir{
							"targetfile": TestFile{Content: string("foobar")},
						},
					},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			var tmb tomb.Tomb
			ctx := tmb.Context(context.Background())

			tempdir, repo, cleanup := prepareTempdirRepoSrc(t, test.src)
			defer cleanup()

			testFS := fs.Track{FS: fs.Local{}}

			arch := New(repo, testFS, Options{})
			arch.runWorkers(ctx, &tmb)

			back := fs.TestChdir(t, tempdir)
			defer back()

			if test.prepare != nil {
				test.prepare(t)
			}

			atree, err := NewTree(testFS, test.targets)
			if err != nil {
				t.Fatal(err)
			}

			tree, err := arch.SaveTree(ctx, "/", atree, nil)
			if err != nil {
				t.Fatal(err)
			}

			treeID, err := repo.SaveTree(ctx, tree)
			if err != nil {
				t.Fatal(err)
			}

			tmb.Kill(nil)
			err = tmb.Wait()
			if err != nil {
				t.Fatal(err)
			}

			err = repo.Flush(ctx)
			if err != nil {
				t.Fatal(err)
			}

			err = repo.SaveIndex(ctx)
			if err != nil {
				t.Fatal(err)
			}

			want := test.want
			if want == nil {
				want = test.src
			}
			TestEnsureTree(ctx, t, "/", repo, treeID, want)
		})
	}
}

func TestArchiverSnapshot(t *testing.T) {
	var tests = []struct {
		name    string
		src     TestDir
		want    TestDir
		chdir   string
		targets []string
	}{
		{
			name: "single-file",
			src: TestDir{
				"foo": TestFile{Content: "foo"},
			},
			targets: []string{"foo"},
		},
		{
			name: "file-current-dir",
			src: TestDir{
				"foo": TestFile{Content: "foo"},
			},
			targets: []string{"./foo"},
		},
		{
			name: "dir",
			src: TestDir{
				"target": TestDir{
					"foo": TestFile{Content: "foo"},
				},
			},
			targets: []string{"target"},
		},
		{
			name: "dir-current-dir",
			src: TestDir{
				"target": TestDir{
					"foo": TestFile{Content: "foo"},
				},
			},
			targets: []string{"./target"},
		},
		{
			name: "content-dir-current-dir",
			src: TestDir{
				"target": TestDir{
					"foo": TestFile{Content: "foo"},
				},
			},
			targets: []string{"./target/."},
		},
		{
			name: "current-dir",
			src: TestDir{
				"target": TestDir{
					"foo": TestFile{Content: "foo"},
				},
			},
			targets: []string{"."},
		},
		{
			name: "subdir",
			src: TestDir{
				"subdir": TestDir{
					"foo": TestFile{Content: "foo"},
					"subsubdir": TestDir{
						"foo": TestFile{Content: "foo in subsubdir"},
					},
				},
				"other": TestFile{Content: "another file"},
			},
			targets: []string{"subdir"},
			want: TestDir{
				"subdir": TestDir{
					"foo": TestFile{Content: "foo"},
					"subsubdir": TestDir{
						"foo": TestFile{Content: "foo in subsubdir"},
					},
				},
			},
		},
		{
			name: "subsubdir",
			src: TestDir{
				"subdir": TestDir{
					"foo": TestFile{Content: "foo"},
					"subsubdir": TestDir{
						"foo": TestFile{Content: "foo in subsubdir"},
					},
				},
				"other": TestFile{Content: "another file"},
			},
			targets: []string{"subdir/subsubdir"},
			want: TestDir{
				"subdir": TestDir{
					"subsubdir": TestDir{
						"foo": TestFile{Content: "foo in subsubdir"},
					},
				},
			},
		},
		{
			name: "parent-dir",
			src: TestDir{
				"subdir": TestDir{
					"foo": TestFile{Content: "foo"},
				},
				"other": TestFile{Content: "another file"},
			},
			chdir:   "subdir",
			targets: []string{".."},
		},
		{
			name: "parent-parent-dir",
			src: TestDir{
				"subdir": TestDir{
					"foo": TestFile{Content: "foo"},
					"subsubdir": TestDir{
						"empty": TestFile{Content: ""},
					},
				},
				"other": TestFile{Content: "another file"},
			},
			chdir:   "subdir/subsubdir",
			targets: []string{"../.."},
		},
		{
			name: "parent-parent-dir-slash",
			src: TestDir{
				"subdir": TestDir{
					"subsubdir": TestDir{
						"foo": TestFile{Content: "foo"},
					},
				},
				"other": TestFile{Content: "another file"},
			},
			chdir:   "subdir/subsubdir",
			targets: []string{"../../"},
			want: TestDir{
				"subdir": TestDir{
					"subsubdir": TestDir{
						"foo": TestFile{Content: "foo"},
					},
				},
				"other": TestFile{Content: "another file"},
			},
		},
		{
			name: "parent-subdir",
			src: TestDir{
				"subdir": TestDir{
					"foo": TestFile{Content: "foo"},
				},
				"other": TestFile{Content: "another file"},
			},
			chdir:   "subdir",
			targets: []string{"../subdir"},
			want: TestDir{
				"subdir": TestDir{
					"foo": TestFile{Content: "foo"},
				},
			},
		},
		{
			name: "parent-parent-dir-subdir",
			src: TestDir{
				"subdir": TestDir{
					"subsubdir": TestDir{
						"foo": TestFile{Content: "foo"},
					},
				},
				"other": TestFile{Content: "another file"},
			},
			chdir:   "subdir/subsubdir",
			targets: []string{"../../subdir/subsubdir"},
			want: TestDir{
				"subdir": TestDir{
					"subsubdir": TestDir{
						"foo": TestFile{Content: "foo"},
					},
				},
			},
		},
		{
			name: "included-multiple1",
			src: TestDir{
				"subdir": TestDir{
					"subsubdir": TestDir{
						"foo": TestFile{Content: "foo"},
					},
					"other": TestFile{Content: "another file"},
				},
			},
			targets: []string{"subdir", "subdir/subsubdir"},
		},
		{
			name: "included-multiple2",
			src: TestDir{
				"subdir": TestDir{
					"subsubdir": TestDir{
						"foo": TestFile{Content: "foo"},
					},
					"other": TestFile{Content: "another file"},
				},
			},
			targets: []string{"subdir/subsubdir", "subdir"},
		},
		{
			name: "collision",
			src: TestDir{
				"subdir": TestDir{
					"foo": TestFile{Content: "foo in subdir"},
					"subsubdir": TestDir{
						"foo": TestFile{Content: "foo in subsubdir"},
					},
				},
				"foo": TestFile{Content: "another file"},
			},
			chdir:   "subdir",
			targets: []string{".", "../foo"},
			want: TestDir{

				"foo": TestFile{Content: "foo in subdir"},
				"subsubdir": TestDir{
					"foo": TestFile{Content: "foo in subsubdir"},
				},
				"foo-1": TestFile{Content: "another file"},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			tempdir, repo, cleanup := prepareTempdirRepoSrc(t, test.src)
			defer cleanup()

			arch := New(repo, fs.Track{FS: fs.Local{}}, Options{})

			chdir := tempdir
			if test.chdir != "" {
				chdir = filepath.Join(chdir, filepath.FromSlash(test.chdir))
			}

			back := fs.TestChdir(t, chdir)
			defer back()

			var targets []string
			for _, target := range test.targets {
				targets = append(targets, os.ExpandEnv(target))
			}

			t.Logf("targets: %v", targets)
			sn, snapshotID, err := arch.Snapshot(ctx, targets, SnapshotOptions{Time: time.Now()})
			if err != nil {
				t.Fatal(err)
			}

			t.Logf("saved as %v", snapshotID.Str())

			want := test.want
			if want == nil {
				want = test.src
			}
			TestEnsureSnapshot(t, repo, snapshotID, want)

			checker.TestCheckRepo(t, repo)

			// check that the snapshot contains the targets with absolute paths
			for i, target := range sn.Paths {
				atarget, err := filepath.Abs(test.targets[i])
				if err != nil {
					t.Fatal(err)
				}

				if target != atarget {
					t.Errorf("wrong path in snapshot: want %v, got %v", atarget, target)
				}
			}
		})
	}
}

func TestArchiverSnapshotSelect(t *testing.T) {
	var tests = []struct {
		name  string
		src   TestDir
		want  TestDir
		selFn SelectFunc
		err   string
	}{
		{
			name: "include-all",
			src: TestDir{
				"work": TestDir{
					"foo":     TestFile{Content: "foo"},
					"foo.txt": TestFile{Content: "foo text file"},
					"subdir": TestDir{
						"other":   TestFile{Content: "other in subdir"},
						"bar.txt": TestFile{Content: "bar.txt in subdir"},
					},
				},
				"other": TestFile{Content: "another file"},
			},
			selFn: func(item string, fi os.FileInfo) bool {
				return true
			},
		},
		{
			name: "exclude-all",
			src: TestDir{
				"work": TestDir{
					"foo":     TestFile{Content: "foo"},
					"foo.txt": TestFile{Content: "foo text file"},
					"subdir": TestDir{
						"other":   TestFile{Content: "other in subdir"},
						"bar.txt": TestFile{Content: "bar.txt in subdir"},
					},
				},
				"other": TestFile{Content: "another file"},
			},
			selFn: func(item string, fi os.FileInfo) bool {
				return false
			},
			err: "snapshot is empty",
		},
		{
			name: "exclude-txt-files",
			src: TestDir{
				"work": TestDir{
					"foo":     TestFile{Content: "foo"},
					"foo.txt": TestFile{Content: "foo text file"},
					"subdir": TestDir{
						"other":   TestFile{Content: "other in subdir"},
						"bar.txt": TestFile{Content: "bar.txt in subdir"},
					},
				},
				"other": TestFile{Content: "another file"},
			},
			want: TestDir{
				"work": TestDir{
					"foo": TestFile{Content: "foo"},
					"subdir": TestDir{
						"other": TestFile{Content: "other in subdir"},
					},
				},
				"other": TestFile{Content: "another file"},
			},
			selFn: func(item string, fi os.FileInfo) bool {
				if filepath.Ext(item) == ".txt" {
					return false
				}
				return true
			},
		},
		{
			name: "exclude-dir",
			src: TestDir{
				"work": TestDir{
					"foo":     TestFile{Content: "foo"},
					"foo.txt": TestFile{Content: "foo text file"},
					"subdir": TestDir{
						"other":   TestFile{Content: "other in subdir"},
						"bar.txt": TestFile{Content: "bar.txt in subdir"},
					},
				},
				"other": TestFile{Content: "another file"},
			},
			want: TestDir{
				"work": TestDir{
					"foo":     TestFile{Content: "foo"},
					"foo.txt": TestFile{Content: "foo text file"},
				},
				"other": TestFile{Content: "another file"},
			},
			selFn: func(item string, fi os.FileInfo) bool {
				if filepath.Base(item) == "subdir" {
					return false
				}
				return true
			},
		},
		{
			name: "select-absolute-paths",
			src: TestDir{
				"foo": TestFile{Content: "foo"},
			},
			selFn: func(item string, fi os.FileInfo) bool {
				return filepath.IsAbs(item)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			tempdir, repo, cleanup := prepareTempdirRepoSrc(t, test.src)
			defer cleanup()

			arch := New(repo, fs.Track{FS: fs.Local{}}, Options{})
			arch.Select = test.selFn

			back := fs.TestChdir(t, tempdir)
			defer back()

			targets := []string{"."}
			_, snapshotID, err := arch.Snapshot(ctx, targets, SnapshotOptions{Time: time.Now()})
			if test.err != "" {
				if err == nil {
					t.Fatalf("expected error not found, got %v, wanted %q", err, test.err)
				}

				if err.Error() != test.err {
					t.Fatalf("unexpected error, want %q, got %q", test.err, err)
				}

				return
			}

			if err != nil {
				t.Fatal(err)
			}

			t.Logf("saved as %v", snapshotID.Str())

			want := test.want
			if want == nil {
				want = test.src
			}
			TestEnsureSnapshot(t, repo, snapshotID, want)

			checker.TestCheckRepo(t, repo)
		})
	}
}

// MockFS keeps track which files are read.
type MockFS struct {
	fs.FS

	m         sync.Mutex
	bytesRead map[string]int // tracks bytes read from all opened files
}

func (m *MockFS) Open(name string) (fs.File, error) {
	f, err := m.FS.Open(name)
	if err != nil {
		return f, err
	}

	return MockFile{File: f, fs: m, filename: name}, nil
}

func (m *MockFS) OpenFile(name string, flag int, perm os.FileMode) (fs.File, error) {
	f, err := m.FS.OpenFile(name, flag, perm)
	if err != nil {
		return f, err
	}

	return MockFile{File: f, fs: m, filename: name}, nil
}

type MockFile struct {
	fs.File
	filename string

	fs *MockFS
}

func (f MockFile) Read(p []byte) (int, error) {
	n, err := f.File.Read(p)
	if n > 0 {
		f.fs.m.Lock()
		f.fs.bytesRead[f.filename] += n
		f.fs.m.Unlock()
	}
	return n, err
}

func TestArchiverParent(t *testing.T) {
	var tests = []struct {
		src  TestDir
		read map[string]int // tracks number of times a file must have been read
	}{
		{
			src: TestDir{
				"targetfile": TestFile{Content: string(restictest.Random(888, 2*1024*1024+5000))},
			},
			read: map[string]int{
				"targetfile": 1,
			},
		},
	}

	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			tempdir, repo, cleanup := prepareTempdirRepoSrc(t, test.src)
			defer cleanup()

			testFS := &MockFS{
				FS:        fs.Track{FS: fs.Local{}},
				bytesRead: make(map[string]int),
			}

			arch := New(repo, testFS, Options{})

			back := fs.TestChdir(t, tempdir)
			defer back()

			_, firstSnapshotID, err := arch.Snapshot(ctx, []string{"."}, SnapshotOptions{Time: time.Now()})
			if err != nil {
				t.Fatal(err)
			}

			t.Logf("first backup saved as %v", firstSnapshotID.Str())
			t.Logf("testfs: %v", testFS)

			// check that all files have been read exactly once
			TestWalkFiles(t, ".", test.src, func(filename string, item interface{}) error {
				file, ok := item.(TestFile)
				if !ok {
					return nil
				}

				n, ok := testFS.bytesRead[filename]
				if !ok {
					t.Fatalf("file %v was not read at all", filename)
				}

				if n != len(file.Content) {
					t.Fatalf("file %v: read %v bytes, wanted %v bytes", filename, n, len(file.Content))
				}
				return nil
			})

			opts := SnapshotOptions{
				Time:           time.Now(),
				ParentSnapshot: firstSnapshotID,
			}
			_, secondSnapshotID, err := arch.Snapshot(ctx, []string{"."}, opts)
			if err != nil {
				t.Fatal(err)
			}

			// check that all files still been read exactly once
			TestWalkFiles(t, ".", test.src, func(filename string, item interface{}) error {
				file, ok := item.(TestFile)
				if !ok {
					return nil
				}

				n, ok := testFS.bytesRead[filename]
				if !ok {
					t.Fatalf("file %v was not read at all", filename)
				}

				if n != len(file.Content) {
					t.Fatalf("file %v: read %v bytes, wanted %v bytes", filename, n, len(file.Content))
				}
				return nil
			})

			t.Logf("second backup saved as %v", secondSnapshotID.Str())
			t.Logf("testfs: %v", testFS)

			checker.TestCheckRepo(t, repo)
		})
	}
}

func TestArchiverErrorReporting(t *testing.T) {
	ignoreErrorForBasename := func(basename string) ErrorFunc {
		return func(item string, fi os.FileInfo, err error) error {
			if filepath.Base(item) == "targetfile" {
				t.Logf("ignoring error for targetfile: %v", err)
				return nil
			}

			t.Errorf("error handler called for unexpected file %v: %v", item, err)
			return err
		}
	}

	chmodUnreadable := func(filename string) func(testing.TB) {
		return func(t testing.TB) {
			if runtime.GOOS == "windows" {
				t.Skip("Skipping this test for windows")
			}

			err := os.Chmod(filepath.FromSlash(filename), 0004)
			if err != nil {
				t.Fatal(err)
			}
		}
	}

	var tests = []struct {
		name      string
		src       TestDir
		want      TestDir
		prepare   func(t testing.TB)
		errFn     ErrorFunc
		mustError bool
	}{
		{
			name: "no-error",
			src: TestDir{
				"targetfile": TestFile{Content: "foobar"},
			},
		},
		{
			name: "file-unreadable",
			src: TestDir{
				"targetfile": TestFile{Content: "foobar"},
			},
			prepare:   chmodUnreadable("targetfile"),
			mustError: true,
		},
		{
			name: "file-unreadable-ignore-error",
			src: TestDir{
				"targetfile": TestFile{Content: "foobar"},
				"other":      TestFile{Content: "xxx"},
			},
			want: TestDir{
				"other": TestFile{Content: "xxx"},
			},
			prepare: chmodUnreadable("targetfile"),
			errFn:   ignoreErrorForBasename("targetfile"),
		},
		{
			name: "file-subdir-unreadable",
			src: TestDir{
				"subdir": TestDir{
					"targetfile": TestFile{Content: "foobar"},
				},
			},
			prepare:   chmodUnreadable("subdir/targetfile"),
			mustError: true,
		},
		{
			name: "file-subdir-unreadable-ignore-error",
			src: TestDir{
				"subdir": TestDir{
					"targetfile": TestFile{Content: "foobar"},
					"other":      TestFile{Content: "xxx"},
				},
			},
			want: TestDir{
				"subdir": TestDir{
					"other": TestFile{Content: "xxx"},
				},
			},
			prepare: chmodUnreadable("subdir/targetfile"),
			errFn:   ignoreErrorForBasename("targetfile"),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			tempdir, repo, cleanup := prepareTempdirRepoSrc(t, test.src)
			defer cleanup()

			back := fs.TestChdir(t, tempdir)
			defer back()

			if test.prepare != nil {
				test.prepare(t)
			}

			arch := New(repo, fs.Track{FS: fs.Local{}}, Options{})
			arch.Error = test.errFn

			_, snapshotID, err := arch.Snapshot(ctx, []string{"."}, SnapshotOptions{Time: time.Now()})
			if test.mustError {
				if err != nil {
					t.Logf("found expected error (%v), skipping further checks", err)
					return
				}

				t.Fatalf("expected error not returned by archiver")
				return
			}

			if err != nil {
				t.Fatalf("unexpected error of type %T found: %v", err, err)
			}

			t.Logf("saved as %v", snapshotID.Str())

			want := test.want
			if want == nil {
				want = test.src
			}
			TestEnsureSnapshot(t, repo, snapshotID, want)

			checker.TestCheckRepo(t, repo)
		})
	}
}

// TrackFS keeps track which files are opened. For some files, an error is injected.
type TrackFS struct {
	fs.FS

	errorOn map[string]error

	opened map[string]uint
	m      sync.Mutex
}

func (m *TrackFS) Open(name string) (fs.File, error) {
	m.m.Lock()
	m.opened[name]++
	m.m.Unlock()

	return m.FS.Open(name)
}

func (m *TrackFS) OpenFile(name string, flag int, perm os.FileMode) (fs.File, error) {
	m.m.Lock()
	m.opened[name]++
	m.m.Unlock()

	return m.FS.OpenFile(name, flag, perm)
}

type failSaveRepo struct {
	restic.Repository
	failAfter int32
	cnt       int32
	err       error
}

func (f *failSaveRepo) SaveBlob(ctx context.Context, t restic.BlobType, buf []byte, id restic.ID) (restic.ID, error) {
	val := atomic.AddInt32(&f.cnt, 1)
	if val >= f.failAfter {
		return restic.ID{}, f.err
	}

	return f.Repository.SaveBlob(ctx, t, buf, id)
}

func TestArchiverAbortEarlyOnError(t *testing.T) {
	var testErr = errors.New("test error")

	var tests = []struct {
		src       TestDir
		wantOpen  map[string]uint
		failAfter uint // error after so many files have been saved to the repo
		err       error
	}{
		{
			src: TestDir{
				"dir": TestDir{
					"bar": TestFile{Content: "foobar"},
					"baz": TestFile{Content: "foobar"},
					"foo": TestFile{Content: "foobar"},
				},
			},
			wantOpen: map[string]uint{
				filepath.FromSlash("dir/bar"): 1,
				filepath.FromSlash("dir/baz"): 1,
				filepath.FromSlash("dir/foo"): 1,
			},
		},
		{
			src: TestDir{
				"dir": TestDir{
					"file1": TestFile{Content: string(restictest.Random(3, 4*1024*1024))},
					"file2": TestFile{Content: string(restictest.Random(3, 4*1024*1024))},
					"file3": TestFile{Content: string(restictest.Random(3, 4*1024*1024))},
					"file4": TestFile{Content: string(restictest.Random(3, 4*1024*1024))},
					"file5": TestFile{Content: string(restictest.Random(3, 4*1024*1024))},
					"file6": TestFile{Content: string(restictest.Random(3, 4*1024*1024))},
					"file7": TestFile{Content: string(restictest.Random(3, 4*1024*1024))},
					"file8": TestFile{Content: string(restictest.Random(3, 4*1024*1024))},
					"file9": TestFile{Content: string(restictest.Random(3, 4*1024*1024))},
				},
			},
			wantOpen: map[string]uint{
				filepath.FromSlash("dir/file1"): 1,
				filepath.FromSlash("dir/file2"): 1,
				filepath.FromSlash("dir/file3"): 1,
				filepath.FromSlash("dir/file7"): 0,
				filepath.FromSlash("dir/file8"): 0,
				filepath.FromSlash("dir/file9"): 0,
			},
			failAfter: 5,
			err:       testErr,
		},
	}

	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			tempdir, repo, cleanup := prepareTempdirRepoSrc(t, test.src)
			defer cleanup()

			back := fs.TestChdir(t, tempdir)
			defer back()

			testFS := &TrackFS{
				FS:     fs.Track{FS: fs.Local{}},
				opened: make(map[string]uint),
			}

			if testFS.errorOn == nil {
				testFS.errorOn = make(map[string]error)
			}

			testRepo := &failSaveRepo{
				Repository: repo,
				failAfter:  int32(test.failAfter),
				err:        test.err,
			}

			arch := New(testRepo, testFS, Options{})

			_, _, err := arch.Snapshot(ctx, []string{"."}, SnapshotOptions{Time: time.Now()})
			if errors.Cause(err) != test.err {
				t.Errorf("expected error (%v) not found, got %v", test.err, errors.Cause(err))
			}

			t.Logf("Snapshot return error: %v", err)

			t.Logf("track fs: %v", testFS.opened)

			for k, v := range test.wantOpen {
				if testFS.opened[k] != v {
					t.Errorf("opened %v %d times, want %d", k, testFS.opened[k], v)
				}
			}
		})
	}
}
