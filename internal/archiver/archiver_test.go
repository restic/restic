package archiver

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/backend/mem"
	"github.com/restic/restic/internal/checker"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/feature"
	"github.com/restic/restic/internal/fs"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
	"golang.org/x/sync/errgroup"
)

func prepareTempdirRepoSrc(t testing.TB, src TestDir) (string, restic.Repository) {
	tempdir := rtest.TempDir(t)
	repo := repository.TestRepository(t)

	TestCreateFiles(t, tempdir, src)

	return tempdir, repo
}

func saveFile(t testing.TB, repo archiverRepo, filename string, filesystem fs.FS) (*restic.Node, ItemStats) {
	wg, ctx := errgroup.WithContext(context.TODO())
	repo.StartPackUploader(ctx, wg)

	arch := New(repo, filesystem, Options{})
	arch.runWorkers(ctx, wg)

	arch.Error = func(item string, err error) error {
		t.Errorf("archiver error for %v: %v", item, err)
		return err
	}

	var (
		completeReadingCallback bool

		completeCallbackNode  *restic.Node
		completeCallbackStats ItemStats
		completeCallback      bool

		startCallback bool
	)

	completeReading := func() {
		completeReadingCallback = true
		if completeCallback {
			t.Error("callbacks called in wrong order")
		}
	}

	complete := func(node *restic.Node, stats ItemStats) {
		completeCallback = true
		completeCallbackNode = node
		completeCallbackStats = stats
	}

	start := func() {
		startCallback = true
	}

	file, err := arch.FS.OpenFile(filename, fs.O_NOFOLLOW, false)
	if err != nil {
		t.Fatal(err)
	}

	res := arch.fileSaver.Save(ctx, "/", filename, file, start, completeReading, complete)

	fnr := res.take(ctx)
	if fnr.err != nil {
		t.Fatal(fnr.err)
	}

	arch.stopWorkers()
	err = repo.Flush(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if err := wg.Wait(); err != nil {
		t.Fatal(err)
	}

	if !startCallback {
		t.Errorf("start callback did not happen")
	}

	if !completeReadingCallback {
		t.Errorf("completeReading callback did not happen")
	}

	if !completeCallback {
		t.Errorf("complete callback did not happen")
	}

	if completeCallbackNode == nil {
		t.Errorf("no node returned for complete callback")
	}

	if completeCallbackNode != nil && !fnr.node.Equals(*completeCallbackNode) {
		t.Errorf("different node returned for complete callback")
	}

	if completeCallbackStats != fnr.stats {
		t.Errorf("different stats return for complete callback, want:\n  %v\ngot:\n  %v", fnr.stats, completeCallbackStats)
	}

	return fnr.node, fnr.stats
}

func TestArchiverSaveFile(t *testing.T) {
	var tests = []TestFile{
		{Content: ""},
		{Content: "foo"},
		{Content: string(rtest.Random(23, 12*1024*1024+1287898))},
	}

	for _, testfile := range tests {
		t.Run("", func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			tempdir, repo := prepareTempdirRepoSrc(t, TestDir{"file": testfile})
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
		{Data: "foo"},
		{Data: string(rtest.Random(23, 12*1024*1024+1287898))},
	}

	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			repo := repository.TestRepository(t)

			ts := time.Now()
			filename := "xx"
			readerFs := &fs.Reader{
				ModTime:    ts,
				Mode:       0123,
				Name:       filename,
				ReadCloser: io.NopCloser(strings.NewReader(test.Data)),
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
		{Content: ""},
		{Content: "foo"},
		{Content: string(rtest.Random(23, 12*1024*1024+1287898))},
	}

	for _, testfile := range tests {
		t.Run("", func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			tempdir, repo := prepareTempdirRepoSrc(t, TestDir{"file": testfile})

			wg, ctx := errgroup.WithContext(ctx)
			repo.StartPackUploader(ctx, wg)

			arch := New(repo, fs.Track{FS: fs.Local{}}, Options{})
			arch.Error = func(item string, err error) error {
				t.Errorf("archiver error for %v: %v", item, err)
				return err
			}
			arch.runWorkers(ctx, wg)
			arch.summary = &Summary{}

			node, excluded, err := arch.save(ctx, "/", filepath.Join(tempdir, "file"), nil)
			if err != nil {
				t.Fatal(err)
			}

			if excluded {
				t.Errorf("Save() excluded the node, that's unexpected")
			}

			fnr := node.take(ctx)
			if fnr.err != nil {
				t.Fatal(fnr.err)
			}

			if fnr.node == nil {
				t.Fatalf("returned node is nil")
			}

			stats := fnr.stats

			arch.stopWorkers()
			err = repo.Flush(ctx)
			if err != nil {
				t.Fatal(err)
			}

			TestEnsureFileContent(ctx, t, repo, "file", fnr.node, testfile)
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
		{Data: "foo"},
		{Data: string(rtest.Random(23, 12*1024*1024+1287898))},
	}

	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			repo := repository.TestRepository(t)

			wg, ctx := errgroup.WithContext(ctx)
			repo.StartPackUploader(ctx, wg)

			ts := time.Now()
			filename := "xx"
			readerFs := &fs.Reader{
				ModTime:    ts,
				Mode:       0123,
				Name:       filename,
				ReadCloser: io.NopCloser(strings.NewReader(test.Data)),
			}

			arch := New(repo, readerFs, Options{})
			arch.Error = func(item string, err error) error {
				t.Errorf("archiver error for %v: %v", item, err)
				return err
			}
			arch.runWorkers(ctx, wg)
			arch.summary = &Summary{}

			node, excluded, err := arch.save(ctx, "/", filename, nil)
			t.Logf("Save returned %v %v", node, err)
			if err != nil {
				t.Fatal(err)
			}

			if excluded {
				t.Errorf("Save() excluded the node, that's unexpected")
			}

			fnr := node.take(ctx)
			if fnr.err != nil {
				t.Fatal(fnr.err)
			}

			if fnr.node == nil {
				t.Fatalf("returned node is nil")
			}

			stats := fnr.stats

			arch.stopWorkers()
			err = repo.Flush(ctx)
			if err != nil {
				t.Fatal(err)
			}

			TestEnsureFileContent(ctx, t, repo, "file", fnr.node, TestFile{Content: test.Data})
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
		Content: string(rtest.Random(23, fileSize)),
	}}

	b.SetBytes(fileSize)

	for i := 0; i < b.N; i++ {
		b.StopTimer()
		tempdir, repo := prepareTempdirRepoSrc(b, d)
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
		b.StartTimer()
	}
}

func BenchmarkArchiverSaveFileLarge(b *testing.B) {
	const fileSize = 40*1024*1024 + 1287898
	d := TestDir{"file": TestFile{
		Content: string(rtest.Random(23, fileSize)),
	}}

	b.SetBytes(fileSize)

	for i := 0; i < b.N; i++ {
		b.StopTimer()
		tempdir, repo := prepareTempdirRepoSrc(b, d)
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
		b.StartTimer()
	}
}

type blobCountingRepo struct {
	archiverRepo

	m     sync.Mutex
	saved map[restic.BlobHandle]uint
}

func (repo *blobCountingRepo) SaveBlob(ctx context.Context, t restic.BlobType, buf []byte, id restic.ID, storeDuplicate bool) (restic.ID, bool, int, error) {
	id, exists, size, err := repo.archiverRepo.SaveBlob(ctx, t, buf, id, storeDuplicate)
	if exists {
		return id, exists, size, err
	}
	h := restic.BlobHandle{ID: id, Type: t}
	repo.m.Lock()
	repo.saved[h]++
	repo.m.Unlock()
	return id, exists, size, err
}

func (repo *blobCountingRepo) SaveTree(ctx context.Context, t *restic.Tree) (restic.ID, error) {
	id, err := restic.SaveTree(ctx, repo.archiverRepo, t)
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
	tempdir := rtest.TempDir(t)

	repo := &blobCountingRepo{
		archiverRepo: repository.TestRepository(t),
		saved:        make(map[restic.BlobHandle]uint),
	}

	data := rtest.Random(23, 512*1024+887898)
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

func chmodTwice(t testing.TB, name string) {
	// POSIX says that ctime is updated "even if the file status does not
	// change", but let's make sure it does change, just in case.
	err := os.Chmod(name, 0700)
	rtest.OK(t, err)

	sleep()

	err = os.Chmod(name, 0600)
	rtest.OK(t, err)
}

func lstat(t testing.TB, name string) *fs.ExtendedFileInfo {
	fi, err := os.Lstat(name)
	if err != nil {
		t.Fatal(err)
	}

	return fs.ExtendedStat(fi)
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

func rename(t testing.TB, oldname, newname string) {
	err := os.Rename(oldname, newname)
	if err != nil {
		t.Fatal(err)
	}
}

func nodeFromFile(t testing.TB, localFs fs.FS, filename string) *restic.Node {
	meta, err := localFs.OpenFile(filename, fs.O_NOFOLLOW, true)
	rtest.OK(t, err)
	node, err := meta.ToNode(false)
	rtest.OK(t, err)
	rtest.OK(t, meta.Close())

	return node
}

// sleep sleeps long enough to ensure a timestamp change.
func sleep() {
	d := 50 * time.Millisecond
	if runtime.GOOS == "darwin" {
		// On older Darwin instances, the file system only supports one second
		// granularity.
		d = 1500 * time.Millisecond
	}
	time.Sleep(d)
}

func TestFileChanged(t *testing.T) {
	var defaultContent = []byte("foobar")

	var tests = []struct {
		Name           string
		SkipForWindows bool
		Content        []byte
		Modify         func(t testing.TB, filename string)
		ChangeIgnore   uint
		SameFile       bool
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
			Name: "new-content-same-timestamp",
			// on Windows, there's no "create time" field users cannot modify,
			// so we're unable to detect if a file has been modified when the
			// timestamps are reset, so we skip this test for Windows
			SkipForWindows: true,
			Modify: func(t testing.TB, filename string) {
				fi, err := os.Stat(filename)
				if err != nil {
					t.Fatal(err)
				}
				extFI := fs.ExtendedStat(fi)
				save(t, filename, bytes.ToUpper(defaultContent))
				sleep()
				setTimestamp(t, filename, extFI.AccessTime, extFI.ModTime)
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
		{
			Name:           "ctime-change",
			Modify:         chmodTwice,
			SameFile:       false,
			SkipForWindows: true, // No ctime on Windows, so this test would fail.
		},
		{
			Name:           "ignore-ctime-change",
			Modify:         chmodTwice,
			ChangeIgnore:   ChangeIgnoreCtime,
			SameFile:       true,
			SkipForWindows: true, // No ctime on Windows, so this test is meaningless.
		},
		{
			Name: "ignore-inode",
			Modify: func(t testing.TB, filename string) {
				fi := lstat(t, filename)
				// First create the new file, then remove the old one,
				// so that the old file retains its inode number.
				tempname := filename + ".old"
				rename(t, filename, tempname)
				save(t, filename, defaultContent)
				remove(t, tempname)
				setTimestamp(t, filename, fi.ModTime, fi.ModTime)
			},
			ChangeIgnore: ChangeIgnoreCtime | ChangeIgnoreInode,
			SameFile:     true,
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			if runtime.GOOS == "windows" && test.SkipForWindows {
				t.Skip("don't run test on Windows")
			}

			tempdir := rtest.TempDir(t)

			filename := filepath.Join(tempdir, "file")
			content := defaultContent
			if test.Content != nil {
				content = test.Content
			}
			save(t, filename, content)

			fs := &fs.Local{}
			fiBefore, err := fs.Lstat(filename)
			rtest.OK(t, err)
			node := nodeFromFile(t, fs, filename)

			if fileChanged(fiBefore, node, 0) {
				t.Fatalf("unchanged file detected as changed")
			}

			test.Modify(t, filename)

			fiAfter := lstat(t, filename)

			if test.SameFile {
				// file should be detected as unchanged
				if fileChanged(fiAfter, node, test.ChangeIgnore) {
					t.Fatalf("unmodified file detected as changed")
				}
			} else {
				// file should be detected as changed
				if !fileChanged(fiAfter, node, test.ChangeIgnore) && !test.SameFile {
					t.Fatalf("modified file detected as unchanged")
				}
			}
		})
	}
}

func TestFilChangedSpecialCases(t *testing.T) {
	tempdir := rtest.TempDir(t)

	filename := filepath.Join(tempdir, "file")
	content := []byte("foobar")
	save(t, filename, content)

	t.Run("nil-node", func(t *testing.T) {
		fi := lstat(t, filename)
		if !fileChanged(fi, nil, 0) {
			t.Fatal("nil node detected as unchanged")
		}
	})

	t.Run("type-change", func(t *testing.T) {
		fi := lstat(t, filename)
		node := nodeFromFile(t, &fs.Local{}, filename)
		node.Type = restic.NodeTypeSymlink
		if !fileChanged(fi, node, 0) {
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
				"targetfile": TestFile{Content: string(rtest.Random(888, 2*1024*1024+5000))},
			},
			target: ".",
			want: TestDir{
				"targetdir": TestDir{
					"targetfile": TestFile{Content: string(rtest.Random(888, 2*1024*1024+5000))},
				},
			},
		},
		{
			src: TestDir{
				"targetdir": TestDir{
					"foo":        TestFile{Content: "foo"},
					"emptyfile":  TestFile{Content: ""},
					"bar":        TestFile{Content: "XXXXXXXXXXXXXXXXXXXXXXXXXXXXXX"},
					"largefile":  TestFile{Content: string(rtest.Random(888, 2*1024*1024+5000))},
					"largerfile": TestFile{Content: string(rtest.Random(234, 5*1024*1024+5000))},
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
			tempdir, repo := prepareTempdirRepoSrc(t, test.src)

			wg, ctx := errgroup.WithContext(context.Background())
			repo.StartPackUploader(ctx, wg)

			testFS := fs.Track{FS: fs.Local{}}
			arch := New(repo, testFS, Options{})
			arch.runWorkers(ctx, wg)
			arch.summary = &Summary{}

			chdir := tempdir
			if test.chdir != "" {
				chdir = filepath.Join(chdir, test.chdir)
			}

			back := rtest.Chdir(t, chdir)
			defer back()

			meta, err := testFS.OpenFile(test.target, fs.O_NOFOLLOW, true)
			rtest.OK(t, err)
			ft, err := arch.saveDir(ctx, "/", test.target, meta, nil, nil)
			rtest.OK(t, err)
			rtest.OK(t, meta.Close())

			fnr := ft.take(ctx)
			node, stats := fnr.node, fnr.stats

			t.Logf("stats: %v", stats)
			if stats.DataSize != 0 {
				t.Errorf("wrong stats returned in DataSize, want 0, got %d", stats.DataSize)
			}
			if stats.DataBlobs != 0 {
				t.Errorf("wrong stats returned in DataBlobs, want 0, got %d", stats.DataBlobs)
			}
			if stats.TreeSize == 0 {
				t.Errorf("wrong stats returned in TreeSize, want > 0, got %d", stats.TreeSize)
			}
			if stats.TreeBlobs <= 0 {
				t.Errorf("wrong stats returned in TreeBlobs, want > 0, got %d", stats.TreeBlobs)
			}

			node.Name = targetNodeName
			tree := &restic.Tree{Nodes: []*restic.Node{node}}
			treeID, err := restic.SaveTree(ctx, repo, tree)
			if err != nil {
				t.Fatal(err)
			}
			arch.stopWorkers()

			err = repo.Flush(ctx)
			if err != nil {
				t.Fatal(err)
			}

			err = wg.Wait()
			if err != nil {
				t.Fatal(err)
			}

			want := test.want
			if want == nil {
				want = test.src
			}
			TestEnsureTree(context.TODO(), t, "/", repo, treeID, want)
		})
	}
}

func TestArchiverSaveDirIncremental(t *testing.T) {
	tempdir := rtest.TempDir(t)

	repo := &blobCountingRepo{
		archiverRepo: repository.TestRepository(t),
		saved:        make(map[restic.BlobHandle]uint),
	}

	appendToFile(t, filepath.Join(tempdir, "testfile"), []byte("foobar"))

	// save the empty directory several times in a row, then have a look if the
	// archiver did save the same tree several times
	for i := 0; i < 5; i++ {
		wg, ctx := errgroup.WithContext(context.TODO())
		repo.StartPackUploader(ctx, wg)

		testFS := fs.Track{FS: fs.Local{}}
		arch := New(repo, testFS, Options{})
		arch.runWorkers(ctx, wg)
		arch.summary = &Summary{}

		meta, err := testFS.OpenFile(tempdir, fs.O_NOFOLLOW, true)
		rtest.OK(t, err)
		ft, err := arch.saveDir(ctx, "/", tempdir, meta, nil, nil)
		rtest.OK(t, err)
		rtest.OK(t, meta.Close())

		fnr := ft.take(ctx)
		node, stats := fnr.node, fnr.stats

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
			if stats.TreeSize == 0 {
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

		arch.stopWorkers()
		err = repo.Flush(ctx)
		if err != nil {
			t.Fatal(err)
		}
		err = wg.Wait()
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

// bothZeroOrNeither fails the test if only one of exp, act is zero.
func bothZeroOrNeither(tb testing.TB, exp, act uint64) {
	tb.Helper()
	if (exp == 0 && act != 0) || (exp != 0 && act == 0) {
		rtest.Equals(tb, exp, act)
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

	// The toplevel directory is not counted in the ItemStats
	var tests = []struct {
		src     TestDir
		prepare func(t testing.TB)
		targets []string
		want    TestDir
		stat    Summary
	}{
		{
			src: TestDir{
				"targetfile": TestFile{Content: string("foobar")},
			},
			targets: []string{"targetfile"},
			want: TestDir{
				"targetfile": TestFile{Content: string("foobar")},
			},
			stat: Summary{
				ItemStats:      ItemStats{1, 6, 32 + 6, 0, 0, 0},
				ProcessedBytes: 6,
				Files:          ChangeStats{1, 0, 0},
				Dirs:           ChangeStats{0, 0, 0},
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
			stat: Summary{
				ItemStats:      ItemStats{1, 6, 32 + 6, 0, 0, 0},
				ProcessedBytes: 6,
				Files:          ChangeStats{1, 0, 0},
				Dirs:           ChangeStats{0, 0, 0},
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
			stat: Summary{
				ItemStats:      ItemStats{0, 0, 0, 1, 0x154, 0x16a},
				ProcessedBytes: 0,
				Files:          ChangeStats{0, 0, 0},
				Dirs:           ChangeStats{1, 0, 0},
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
			stat: Summary{
				ItemStats:      ItemStats{1, 6, 32 + 6, 3, 0x47f, 0x4c1},
				ProcessedBytes: 6,
				Files:          ChangeStats{1, 0, 0},
				Dirs:           ChangeStats{3, 0, 0},
			},
		},
	}

	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			tempdir, repo := prepareTempdirRepoSrc(t, test.src)

			testFS := fs.Track{FS: fs.Local{}}

			arch := New(repo, testFS, Options{})

			wg, ctx := errgroup.WithContext(context.TODO())
			repo.StartPackUploader(ctx, wg)

			arch.runWorkers(ctx, wg)
			arch.summary = &Summary{}

			back := rtest.Chdir(t, tempdir)
			defer back()

			if test.prepare != nil {
				test.prepare(t)
			}

			atree, err := newTree(testFS, test.targets)
			if err != nil {
				t.Fatal(err)
			}

			fn, _, err := arch.saveTree(ctx, "/", atree, nil, nil)
			if err != nil {
				t.Fatal(err)
			}

			fnr := fn.take(context.TODO())
			if fnr.err != nil {
				t.Fatal(fnr.err)
			}

			treeID := *fnr.node.Subtree

			arch.stopWorkers()
			err = repo.Flush(ctx)
			if err != nil {
				t.Fatal(err)
			}
			err = wg.Wait()
			if err != nil {
				t.Fatal(err)
			}

			want := test.want
			if want == nil {
				want = test.src
			}
			TestEnsureTree(context.TODO(), t, "/", repo, treeID, want)
			stat := arch.summary
			bothZeroOrNeither(t, uint64(test.stat.DataBlobs), uint64(stat.DataBlobs))
			bothZeroOrNeither(t, uint64(test.stat.TreeBlobs), uint64(stat.TreeBlobs))
			bothZeroOrNeither(t, test.stat.DataSize, stat.DataSize)
			bothZeroOrNeither(t, test.stat.DataSizeInRepo, stat.DataSizeInRepo)
			bothZeroOrNeither(t, test.stat.TreeSizeInRepo, stat.TreeSizeInRepo)
			rtest.Equals(t, test.stat.ProcessedBytes, stat.ProcessedBytes)
			rtest.Equals(t, test.stat.Files, stat.Files)
			rtest.Equals(t, test.stat.Dirs, stat.Dirs)
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

			tempdir, repo := prepareTempdirRepoSrc(t, test.src)

			arch := New(repo, fs.Track{FS: fs.Local{}}, Options{})

			chdir := tempdir
			if test.chdir != "" {
				chdir = filepath.Join(chdir, filepath.FromSlash(test.chdir))
			}

			back := rtest.Chdir(t, chdir)
			defer back()

			var targets []string
			for _, target := range test.targets {
				targets = append(targets, os.ExpandEnv(target))
			}

			t.Logf("targets: %v", targets)
			sn, snapshotID, _, err := arch.Snapshot(ctx, targets, SnapshotOptions{Time: time.Now()})
			if err != nil {
				t.Fatal(err)
			}

			t.Logf("saved as %v", snapshotID.Str())

			want := test.want
			if want == nil {
				want = test.src
			}
			TestEnsureSnapshot(t, repo, snapshotID, want)

			checker.TestCheckRepo(t, repo, false)

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

func TestResolveRelativeTargetsSpecial(t *testing.T) {
	var tests = []struct {
		name     string
		targets  []string
		expected []string
		win      bool
	}{
		{
			name:     "basic relative path",
			targets:  []string{filepath.FromSlash("some/path")},
			expected: []string{filepath.FromSlash("some/path")},
		},
		{
			name:     "partial relative path",
			targets:  []string{filepath.FromSlash("../some/path")},
			expected: []string{filepath.FromSlash("../some/path")},
		},
		{
			name:     "basic absolute path",
			targets:  []string{filepath.FromSlash("/some/path")},
			expected: []string{filepath.FromSlash("/some/path")},
		},
		{
			name:     "volume name",
			targets:  []string{"C:"},
			expected: []string{"C:\\"},
			win:      true,
		},
		{
			name:     "volume root path",
			targets:  []string{"C:\\"},
			expected: []string{"C:\\"},
			win:      true,
		},
		{
			name:     "UNC path",
			targets:  []string{"\\\\server\\volume"},
			expected: []string{"\\\\server\\volume\\"},
			win:      true,
		},
		{
			name:     "UNC path with trailing slash",
			targets:  []string{"\\\\server\\volume\\"},
			expected: []string{"\\\\server\\volume\\"},
			win:      true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.win && runtime.GOOS != "windows" {
				t.Skip("skip test on unix")
			}

			targets, err := resolveRelativeTargets(&fs.Local{}, test.targets)
			rtest.OK(t, err)
			rtest.Equals(t, test.expected, targets)
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
			selFn: func(item string, fi *fs.ExtendedFileInfo, _ fs.FS) bool {
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
			selFn: func(item string, fi *fs.ExtendedFileInfo, _ fs.FS) bool {
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
			selFn: func(item string, fi *fs.ExtendedFileInfo, _ fs.FS) bool {
				return filepath.Ext(item) != ".txt"
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
			selFn: func(item string, fi *fs.ExtendedFileInfo, fs fs.FS) bool {
				return fs.Base(item) != "subdir"
			},
		},
		{
			name: "select-absolute-paths",
			src: TestDir{
				"foo": TestFile{Content: "foo"},
			},
			selFn: func(item string, fi *fs.ExtendedFileInfo, fs fs.FS) bool {
				return fs.IsAbs(item)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			tempdir, repo := prepareTempdirRepoSrc(t, test.src)

			arch := New(repo, fs.Track{FS: fs.Local{}}, Options{})
			arch.Select = test.selFn

			back := rtest.Chdir(t, tempdir)
			defer back()

			targets := []string{"."}
			_, snapshotID, _, err := arch.Snapshot(ctx, targets, SnapshotOptions{Time: time.Now()})
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

			checker.TestCheckRepo(t, repo, false)
		})
	}
}

// MockFS keeps track which files are read.
type MockFS struct {
	fs.FS

	m         sync.Mutex
	bytesRead map[string]int // tracks bytes read from all opened files
}

func (m *MockFS) OpenFile(name string, flag int, metadataOnly bool) (fs.File, error) {
	f, err := m.FS.OpenFile(name, flag, metadataOnly)
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

func checkSnapshotStats(t *testing.T, sn *restic.Snapshot, stat Summary) {
	t.Helper()
	rtest.Equals(t, stat.BackupStart, sn.Summary.BackupStart, "BackupStart")
	// BackupEnd is set to time.Now() and can't be compared to a fixed value
	rtest.Equals(t, stat.Files.New, sn.Summary.FilesNew, "FilesNew")
	rtest.Equals(t, stat.Files.Changed, sn.Summary.FilesChanged, "FilesChanged")
	rtest.Equals(t, stat.Files.Unchanged, sn.Summary.FilesUnmodified, "FilesUnmodified")
	rtest.Equals(t, stat.Dirs.New, sn.Summary.DirsNew, "DirsNew")
	rtest.Equals(t, stat.Dirs.Changed, sn.Summary.DirsChanged, "DirsChanged")
	rtest.Equals(t, stat.Dirs.Unchanged, sn.Summary.DirsUnmodified, "DirsUnmodified")
	rtest.Equals(t, stat.ProcessedBytes, sn.Summary.TotalBytesProcessed, "TotalBytesProcessed")
	rtest.Equals(t, stat.Files.New+stat.Files.Changed+stat.Files.Unchanged, sn.Summary.TotalFilesProcessed, "TotalFilesProcessed")
	bothZeroOrNeither(t, uint64(stat.DataBlobs), uint64(sn.Summary.DataBlobs))
	bothZeroOrNeither(t, uint64(stat.TreeBlobs), uint64(sn.Summary.TreeBlobs))
	bothZeroOrNeither(t, uint64(stat.DataSize+stat.TreeSize), uint64(sn.Summary.DataAdded))
	bothZeroOrNeither(t, uint64(stat.DataSizeInRepo+stat.TreeSizeInRepo), uint64(sn.Summary.DataAddedPacked))
}

func TestArchiverParent(t *testing.T) {
	var tests = []struct {
		src         TestDir
		modify      func(path string)
		statInitial Summary
		statSecond  Summary
	}{
		{
			src: TestDir{
				"targetfile": TestFile{Content: string(rtest.Random(888, 2*1024*1024+5000))},
			},
			statInitial: Summary{
				Files:          ChangeStats{1, 0, 0},
				Dirs:           ChangeStats{0, 0, 0},
				ProcessedBytes: 2102152,
				ItemStats:      ItemStats{3, 0x201593, 0x201632, 1, 0, 0},
			},
			statSecond: Summary{
				Files:          ChangeStats{0, 0, 1},
				Dirs:           ChangeStats{0, 0, 0},
				ProcessedBytes: 2102152,
			},
		},
		{
			src: TestDir{
				"targetDir": TestDir{
					"targetfile":  TestFile{Content: string(rtest.Random(888, 1234))},
					"targetfile2": TestFile{Content: string(rtest.Random(888, 1235))},
				},
			},
			statInitial: Summary{
				Files:          ChangeStats{2, 0, 0},
				Dirs:           ChangeStats{1, 0, 0},
				ProcessedBytes: 2469,
				ItemStats:      ItemStats{2, 0xe1c, 0xcd9, 2, 0, 0},
			},
			statSecond: Summary{
				Files:          ChangeStats{0, 0, 2},
				Dirs:           ChangeStats{0, 0, 1},
				ProcessedBytes: 2469,
			},
		},
		{
			src: TestDir{
				"targetDir": TestDir{
					"targetfile": TestFile{Content: string(rtest.Random(888, 1234))},
				},
				"targetfile2": TestFile{Content: string(rtest.Random(888, 1235))},
			},
			modify: func(path string) {
				remove(t, filepath.Join(path, "targetDir", "targetfile"))
				save(t, filepath.Join(path, "targetfile2"), []byte("foobar"))
			},
			statInitial: Summary{
				Files:          ChangeStats{2, 0, 0},
				Dirs:           ChangeStats{1, 0, 0},
				ProcessedBytes: 2469,
				ItemStats:      ItemStats{2, 0xe13, 0xcf8, 2, 0, 0},
			},
			statSecond: Summary{
				Files:          ChangeStats{0, 1, 0},
				Dirs:           ChangeStats{0, 1, 0},
				ProcessedBytes: 6,
				ItemStats:      ItemStats{1, 0x305, 0x233, 2, 0, 0},
			},
		},
	}

	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			tempdir, repo := prepareTempdirRepoSrc(t, test.src)

			testFS := &MockFS{
				FS:        fs.Track{FS: fs.Local{}},
				bytesRead: make(map[string]int),
			}

			arch := New(repo, testFS, Options{})

			back := rtest.Chdir(t, tempdir)
			defer back()

			firstSnapshot, firstSnapshotID, summary, err := arch.Snapshot(ctx, []string{"."}, SnapshotOptions{Time: time.Now()})
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
			rtest.Equals(t, test.statInitial.Files, summary.Files)
			rtest.Equals(t, test.statInitial.Dirs, summary.Dirs)
			rtest.Equals(t, test.statInitial.ProcessedBytes, summary.ProcessedBytes)
			checkSnapshotStats(t, firstSnapshot, test.statInitial)

			if test.modify != nil {
				test.modify(tempdir)
			}

			opts := SnapshotOptions{
				Time:           time.Now(),
				ParentSnapshot: firstSnapshot,
			}
			testFS.bytesRead = map[string]int{}
			secondSnapshot, secondSnapshotID, summary, err := arch.Snapshot(ctx, []string{"."}, opts)
			if err != nil {
				t.Fatal(err)
			}

			if test.modify == nil {
				// check that no files were read this time
				rtest.Equals(t, map[string]int{}, testFS.bytesRead)
			}
			rtest.Equals(t, test.statSecond.Files, summary.Files)
			rtest.Equals(t, test.statSecond.Dirs, summary.Dirs)
			rtest.Equals(t, test.statSecond.ProcessedBytes, summary.ProcessedBytes)
			checkSnapshotStats(t, secondSnapshot, test.statSecond)

			t.Logf("second backup saved as %v", secondSnapshotID.Str())
			t.Logf("testfs: %v", testFS)

			checker.TestCheckRepo(t, repo, false)
		})
	}
}

func TestArchiverErrorReporting(t *testing.T) {
	ignoreErrorForBasename := func(basename string) ErrorFunc {
		return func(item string, err error) error {
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

			tempdir, repo := prepareTempdirRepoSrc(t, test.src)

			back := rtest.Chdir(t, tempdir)
			defer back()

			if test.prepare != nil {
				test.prepare(t)
			}

			arch := New(repo, fs.Track{FS: fs.Local{}}, Options{})
			arch.Error = test.errFn

			_, snapshotID, _, err := arch.Snapshot(ctx, []string{"."}, SnapshotOptions{Time: time.Now()})
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

			checker.TestCheckRepo(t, repo, false)
		})
	}
}

type noCancelBackend struct {
	backend.Backend
}

func (c *noCancelBackend) Remove(_ context.Context, h backend.Handle) error {
	return c.Backend.Remove(context.Background(), h)
}

func (c *noCancelBackend) Save(_ context.Context, h backend.Handle, rd backend.RewindReader) error {
	return c.Backend.Save(context.Background(), h, rd)
}

func (c *noCancelBackend) Load(_ context.Context, h backend.Handle, length int, offset int64, fn func(rd io.Reader) error) error {
	return c.Backend.Load(context.Background(), h, length, offset, fn)
}

func (c *noCancelBackend) Stat(_ context.Context, h backend.Handle) (backend.FileInfo, error) {
	return c.Backend.Stat(context.Background(), h)
}

func (c *noCancelBackend) List(_ context.Context, t backend.FileType, fn func(backend.FileInfo) error) error {
	return c.Backend.List(context.Background(), t, fn)
}

func (c *noCancelBackend) Delete(_ context.Context) error {
	return c.Backend.Delete(context.Background())
}

func TestArchiverContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	tempdir := rtest.TempDir(t)
	TestCreateFiles(t, tempdir, TestDir{
		"targetfile": TestFile{Content: "foobar"},
	})

	// Ensure that the archiver itself reports the canceled context and not just the backend
	repo, _ := repository.TestRepositoryWithBackend(t, &noCancelBackend{mem.New()}, 0, repository.Options{})

	back := rtest.Chdir(t, tempdir)
	defer back()

	arch := New(repo, fs.Track{FS: fs.Local{}}, Options{})

	_, snapshotID, _, err := arch.Snapshot(ctx, []string{"."}, SnapshotOptions{Time: time.Now()})

	if err != nil {
		t.Logf("found expected error (%v)", err)
		return
	}
	if snapshotID.IsNull() {
		t.Fatalf("no error returned but found null id")
	}

	t.Fatalf("expected error not returned by archiver")
}

// TrackFS keeps track which files are opened. For some files, an error is injected.
type TrackFS struct {
	fs.FS

	errorOn map[string]error

	opened map[string]uint
	m      sync.Mutex
}

func (m *TrackFS) OpenFile(name string, flag int, metadataOnly bool) (fs.File, error) {
	m.m.Lock()
	m.opened[name]++
	m.m.Unlock()

	return m.FS.OpenFile(name, flag, metadataOnly)
}

type failSaveRepo struct {
	archiverRepo
	failAfter int32
	cnt       int32
	err       error
}

func (f *failSaveRepo) SaveBlob(ctx context.Context, t restic.BlobType, buf []byte, id restic.ID, storeDuplicate bool) (restic.ID, bool, int, error) {
	val := atomic.AddInt32(&f.cnt, 1)
	if val >= f.failAfter {
		return restic.Hash(buf), false, 0, f.err
	}

	return f.archiverRepo.SaveBlob(ctx, t, buf, id, storeDuplicate)
}

func TestArchiverAbortEarlyOnError(t *testing.T) {
	var testErr = errors.New("test error")

	var tests = []struct {
		src       TestDir
		wantOpen  map[string]uint
		failAfter uint // error after so many blobs have been saved to the repo
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
					"file0": TestFile{Content: string(rtest.Random(0, 1024))},
					"file1": TestFile{Content: string(rtest.Random(1, 1024))},
					"file2": TestFile{Content: string(rtest.Random(2, 1024))},
					"file3": TestFile{Content: string(rtest.Random(3, 1024))},
					"file4": TestFile{Content: string(rtest.Random(4, 1024))},
					"file5": TestFile{Content: string(rtest.Random(5, 1024))},
					"file6": TestFile{Content: string(rtest.Random(6, 1024))},
					"file7": TestFile{Content: string(rtest.Random(7, 1024))},
					"file8": TestFile{Content: string(rtest.Random(8, 1024))},
					"file9": TestFile{Content: string(rtest.Random(9, 1024))},
				},
			},
			wantOpen: map[string]uint{
				filepath.FromSlash("dir/file0"): 1,
				filepath.FromSlash("dir/file1"): 1,
				filepath.FromSlash("dir/file2"): 1,
				filepath.FromSlash("dir/file3"): 1,
				filepath.FromSlash("dir/file8"): 0,
				filepath.FromSlash("dir/file9"): 0,
			},
			// fails after four to seven files were opened, as the ReadConcurrency allows for
			// two queued files and SaveBlobConcurrency for one blob queued for saving.
			failAfter: 4,
			err:       testErr,
		},
	}

	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			tempdir, repo := prepareTempdirRepoSrc(t, test.src)

			back := rtest.Chdir(t, tempdir)
			defer back()

			testFS := &TrackFS{
				FS:     fs.Track{FS: fs.Local{}},
				opened: make(map[string]uint),
			}

			if testFS.errorOn == nil {
				testFS.errorOn = make(map[string]error)
			}

			testRepo := &failSaveRepo{
				archiverRepo: repo,
				failAfter:    int32(test.failAfter),
				err:          test.err,
			}

			// at most two files may be queued
			arch := New(testRepo, testFS, Options{
				ReadConcurrency:     2,
				SaveBlobConcurrency: 1,
			})

			_, _, _, err := arch.Snapshot(ctx, []string{"."}, SnapshotOptions{Time: time.Now()})
			if !errors.Is(err, test.err) {
				t.Errorf("expected error (%v) not found, got %v", test.err, err)
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

func snapshot(t testing.TB, repo archiverRepo, fs fs.FS, parent *restic.Snapshot, filename string) (*restic.Snapshot, *restic.Node) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	arch := New(repo, fs, Options{})

	sopts := SnapshotOptions{
		Time:           time.Now(),
		ParentSnapshot: parent,
	}
	snapshot, _, _, err := arch.Snapshot(ctx, []string{filename}, sopts)
	if err != nil {
		t.Fatal(err)
	}

	tree, err := restic.LoadTree(ctx, repo, *snapshot.Tree)
	if err != nil {
		t.Fatal(err)
	}

	node := tree.Find(filename)
	if node == nil {
		t.Fatalf("unable to find node for testfile in snapshot")
	}

	return snapshot, node
}

type overrideFS struct {
	fs.FS
	overrideFI    *fs.ExtendedFileInfo
	resetFIOnRead bool
	overrideNode  *restic.Node
	overrideErr   error
}

func (m *overrideFS) OpenFile(name string, flag int, metadataOnly bool) (fs.File, error) {
	f, err := m.FS.OpenFile(name, flag, metadataOnly)
	if err != nil {
		return f, err
	}

	if filepath.Base(name) == "testfile" || filepath.Base(name) == "testdir" {
		return &overrideFile{f, m}, nil
	}
	return f, nil
}

type overrideFile struct {
	fs.File
	ofs *overrideFS
}

func (f overrideFile) Stat() (*fs.ExtendedFileInfo, error) {
	if f.ofs.overrideFI == nil {
		return f.File.Stat()
	}
	return f.ofs.overrideFI, nil

}

func (f overrideFile) MakeReadable() error {
	if f.ofs.resetFIOnRead {
		f.ofs.overrideFI = nil
	}
	return f.File.MakeReadable()
}

func (f overrideFile) ToNode(ignoreXattrListError bool) (*restic.Node, error) {
	if f.ofs.overrideNode == nil {
		return f.File.ToNode(ignoreXattrListError)
	}
	return f.ofs.overrideNode, f.ofs.overrideErr
}

// used by wrapFileInfo, use untyped const in order to avoid having a version
// of wrapFileInfo for each OS
const (
	mockFileInfoMode = 0400
	mockFileInfoUID  = 51234
	mockFileInfoGID  = 51235
)

func TestMetadataChanged(t *testing.T) {
	defer feature.TestSetFlag(t, feature.Flag, feature.DeviceIDForHardlinks, true)()

	files := TestDir{
		"testfile": TestFile{
			Content: "foo bar test file",
		},
	}

	tempdir, repo := prepareTempdirRepoSrc(t, files)

	back := rtest.Chdir(t, tempdir)
	defer back()

	// get metadata
	fi := lstat(t, "testfile")
	localFS := &fs.Local{}
	meta, err := localFS.OpenFile("testfile", fs.O_NOFOLLOW, true)
	rtest.OK(t, err)
	want, err := meta.ToNode(false)
	rtest.OK(t, err)
	rtest.OK(t, meta.Close())

	fs := &overrideFS{
		FS:           localFS,
		overrideFI:   fi,
		overrideNode: &restic.Node{},
	}
	*fs.overrideNode = *want

	sn, node2 := snapshot(t, repo, fs, nil, "testfile")

	// set some values so we can then compare the nodes
	want.DeviceID = 0
	want.Content = node2.Content
	want.Path = ""
	if len(want.ExtendedAttributes) == 0 {
		want.ExtendedAttributes = nil
	}

	want.AccessTime = want.ModTime

	// make sure that metadata was recorded successfully
	if !cmp.Equal(want, node2) {
		t.Fatalf("metadata does not match:\n%v", cmp.Diff(want, node2))
	}

	// modify the mode and UID/GID
	modFI := *fi
	modFI.Mode = mockFileInfoMode
	if runtime.GOOS != "windows" {
		modFI.UID = mockFileInfoUID
		modFI.GID = mockFileInfoGID
	}

	fs.overrideFI = &modFI
	rtest.Assert(t, !fileChanged(fs.overrideFI, node2, 0), "testfile must not be considered as changed")

	// set the override values in the 'want' node which
	want.Mode = mockFileInfoMode
	// ignore UID and GID on Windows
	if runtime.GOOS != "windows" {
		want.UID = mockFileInfoUID
		want.GID = mockFileInfoGID
	}
	// update mock node accordingly
	fs.overrideNode.Mode = want.Mode
	fs.overrideNode.UID = want.UID
	fs.overrideNode.GID = want.GID

	// make another snapshot
	_, node3 := snapshot(t, repo, fs, sn, "testfile")

	// make sure that metadata was recorded successfully
	if !cmp.Equal(want, node3) {
		t.Fatalf("metadata does not match:\n%v", cmp.Diff(want, node3))
	}

	// make sure the content matches
	TestEnsureFileContent(context.Background(), t, repo, "testfile", node3, files["testfile"].(TestFile))

	checker.TestCheckRepo(t, repo, false)
}

func TestRacyFileTypeSwap(t *testing.T) {
	files := TestDir{
		"testfile": TestFile{
			Content: "foo bar test file",
		},
		"testdir": TestDir{},
	}

	for _, dirError := range []bool{false, true} {
		desc := "file changed type"
		if dirError {
			desc = "dir changed type"
		}
		t.Run(desc, func(t *testing.T) {
			tempdir, repo := prepareTempdirRepoSrc(t, files)

			back := rtest.Chdir(t, tempdir)
			defer back()

			// get metadata of current folder
			var fakeName, realName string
			if dirError {
				// lstat claims this is a directory, but it's actually a file
				fakeName = "testdir"
				realName = "testfile"
			} else {
				fakeName = "testfile"
				realName = "testdir"
			}
			fakeFI := lstat(t, fakeName)
			tempfile := filepath.Join(tempdir, realName)

			statfs := &overrideFS{
				FS:            fs.Local{},
				overrideFI:    fakeFI,
				resetFIOnRead: true,
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			wg, ctx := errgroup.WithContext(ctx)
			repo.StartPackUploader(ctx, wg)

			arch := New(repo, fs.Track{FS: statfs}, Options{})
			arch.Error = func(item string, err error) error {
				t.Logf("archiver error as expected for %v: %v", item, err)
				return err
			}
			arch.runWorkers(ctx, wg)

			// fs.Track will panic if the file was not closed
			_, excluded, err := arch.save(ctx, "/", tempfile, nil)
			rtest.Assert(t, err != nil && strings.Contains(err.Error(), "changed type, refusing to archive"), "save() returned wrong error: %v", err)
			tpe := "file"
			if dirError {
				tpe = "directory"
			}
			rtest.Assert(t, strings.Contains(err.Error(), tpe+" "), "unexpected item type in error: %v", err)
			rtest.Assert(t, !excluded, "Save() excluded the node, that's unexpected")
		})
	}
}

type mockToNoder struct {
	node *restic.Node
	err  error
}

func (m *mockToNoder) ToNode(_ bool) (*restic.Node, error) {
	return m.node, m.err
}

func TestMetadataBackupErrorFiltering(t *testing.T) {
	tempdir := t.TempDir()
	filename := filepath.Join(tempdir, "file")
	repo := repository.TestRepository(t)

	arch := New(repo, fs.Local{}, Options{})

	var filteredErr error
	replacementErr := fmt.Errorf("replacement")
	arch.Error = func(item string, err error) error {
		filteredErr = err
		return replacementErr
	}

	nonExistNoder := &mockToNoder{
		node: &restic.Node{Type: restic.NodeTypeFile},
		err:  fmt.Errorf("not found"),
	}

	// check that errors from reading extended metadata are properly filtered
	node, err := arch.nodeFromFileInfo("file", filename+"invalid", nonExistNoder, false)
	rtest.Assert(t, node != nil, "node is missing")
	rtest.Assert(t, err == replacementErr, "expected %v got %v", replacementErr, err)
	rtest.Assert(t, filteredErr != nil, "missing inner error")

	// check that errors from reading irregular file are not filtered
	filteredErr = nil
	nonExistNoder = &mockToNoder{
		node: &restic.Node{Type: restic.NodeTypeIrregular},
		err:  fmt.Errorf(`unsupported file type "irregular"`),
	}
	node, err = arch.nodeFromFileInfo("file", filename, nonExistNoder, false)
	rtest.Assert(t, node != nil, "node is missing")
	rtest.Assert(t, filteredErr == nil, "error for irregular node should not have been filtered")
	rtest.Assert(t, strings.Contains(err.Error(), "irregular"), "unexpected error %q does not warn about irregular file mode", err)
}

func TestIrregularFile(t *testing.T) {
	files := TestDir{
		"testfile": TestFile{
			Content: "foo bar test file",
		},
	}
	tempdir, repo := prepareTempdirRepoSrc(t, files)

	back := rtest.Chdir(t, tempdir)
	defer back()

	tempfile := filepath.Join(tempdir, "testfile")
	fi := lstat(t, "testfile")
	// patch mode to irregular
	fi.Mode = (fi.Mode &^ os.ModeType) | os.ModeIrregular

	override := &overrideFS{
		FS:         fs.Local{},
		overrideFI: fi,
		overrideNode: &restic.Node{
			Type: restic.NodeTypeIrregular,
		},
		overrideErr: fmt.Errorf(`unsupported file type "irregular"`),
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	arch := New(repo, fs.Track{FS: override}, Options{})
	_, excluded, err := arch.save(ctx, "/", tempfile, nil)
	if err == nil {
		t.Fatalf("Save() should have failed")
	}
	rtest.Assert(t, strings.Contains(err.Error(), "irregular"), "unexpected error %q does not warn about irregular file mode", err)

	if excluded {
		t.Errorf("Save() excluded the node, that's unexpected")
	}
}

type missingFS struct {
	fs.FS
	errorOnOpen bool
}

func (fs *missingFS) OpenFile(name string, flag int, metadataOnly bool) (fs.File, error) {
	if fs.errorOnOpen {
		return nil, os.ErrNotExist
	}

	return &missingFile{}, nil
}

type missingFile struct {
	fs.File
}

func (f *missingFile) Stat() (*fs.ExtendedFileInfo, error) {
	return nil, os.ErrNotExist
}

func (f *missingFile) Close() error {
	// prevent segfault in test
	return nil
}

func TestDisappearedFile(t *testing.T) {
	tempdir, repo := prepareTempdirRepoSrc(t, TestDir{})

	back := rtest.Chdir(t, tempdir)
	defer back()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// depending on the underlying FS implementation a missing file may be detected by OpenFile or
	// the subsequent file.Stat() call. Thus test both cases.
	for _, errorOnOpen := range []bool{false, true} {
		arch := New(repo, fs.Track{FS: &missingFS{FS: &fs.Local{}, errorOnOpen: errorOnOpen}}, Options{})
		_, excluded, err := arch.save(ctx, "/", filepath.Join(tempdir, "testdir"), nil)
		rtest.OK(t, err)
		rtest.Assert(t, excluded, "testfile should have been excluded")
	}
}
