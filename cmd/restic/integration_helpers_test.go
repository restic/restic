package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/backend/retry"
	"github.com/restic/restic/internal/data"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/options"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
	"github.com/restic/restic/internal/ui"
	"github.com/restic/restic/internal/ui/termstatus"
)

type dirEntry struct {
	path string
	fi   os.FileInfo
	link uint64
}

func walkDir(t testing.TB, dir string) <-chan *dirEntry {
	ch := make(chan *dirEntry, 100)

	go func() {
		err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				t.Logf("error: %v\n", err)
				return nil
			}

			name, err := filepath.Rel(dir, path)
			if err != nil {
				t.Logf("error: %v\n", err)
				return nil
			}

			ch <- &dirEntry{
				path: name,
				fi:   info,
				link: nlink(info),
			}

			return nil
		})

		if err != nil {
			t.Logf("Walk() error: %v\n", err)
		}

		close(ch)
	}()

	// first element is root
	<-ch

	return ch
}

func isSymlink(fi os.FileInfo) bool {
	mode := fi.Mode() & (os.ModeType | os.ModeCharDevice)
	return mode == os.ModeSymlink
}

func sameModTime(fi1, fi2 os.FileInfo) bool {
	switch runtime.GOOS {
	case "darwin", "freebsd", "openbsd", "netbsd", "solaris":
		if isSymlink(fi1) && isSymlink(fi2) {
			return true
		}
	}

	return fi1.ModTime().Equal(fi2.ModTime())
}

// directoriesContentsDiff returns a diff between both directories. If these
// contain exactly the same contents, then the diff is an empty string.
func directoriesContentsDiff(t testing.TB, dir1, dir2 string) string {
	var out bytes.Buffer
	ch1 := walkDir(t, dir1)
	ch2 := walkDir(t, dir2)

	var a, b *dirEntry
	for {
		var ok bool

		if ch1 != nil && a == nil {
			a, ok = <-ch1
			if !ok {
				ch1 = nil
			}
		}

		if ch2 != nil && b == nil {
			b, ok = <-ch2
			if !ok {
				ch2 = nil
			}
		}

		if ch1 == nil && ch2 == nil {
			break
		}

		if ch1 == nil {
			fmt.Fprintf(&out, "+%v\n", b.path)
		} else if ch2 == nil {
			fmt.Fprintf(&out, "-%v\n", a.path)
		} else if !a.equals(&out, b) {
			if a.path < b.path {
				fmt.Fprintf(&out, "-%v\n", a.path)
				a = nil
				continue
			} else if a.path > b.path {
				fmt.Fprintf(&out, "+%v\n", b.path)
				b = nil
				continue
			}
			fmt.Fprintf(&out, "%%%v\n", a.path)
		}

		a, b = nil, nil
	}

	return out.String()
}

type dirStat struct {
	files, dirs, other uint
	size               uint64
}

func isFile(fi os.FileInfo) bool {
	return fi.Mode()&(os.ModeType|os.ModeCharDevice) == 0
}

// dirStats walks dir and collects stats.
func dirStats(t testing.TB, dir string) (stat dirStat) {
	for entry := range walkDir(t, dir) {
		if isFile(entry.fi) {
			stat.files++
			stat.size += uint64(entry.fi.Size())
			continue
		}

		if entry.fi.IsDir() {
			stat.dirs++
			continue
		}

		stat.other++
	}

	return stat
}

type testEnvironment struct {
	base, cache, repo, mountpoint, testdata string
	gopts                                   GlobalOptions
}

type logOutputter struct {
	t testing.TB
}

func (l *logOutputter) Write(p []byte) (n int, err error) {
	l.t.Helper()
	l.t.Log(strings.TrimSuffix(string(p), "\n"))
	return len(p), nil
}

// withTestEnvironment creates a test environment and returns a cleanup
// function which removes it.
func withTestEnvironment(t testing.TB) (env *testEnvironment, cleanup func()) {
	if !rtest.RunIntegrationTest {
		t.Skip("integration tests disabled")
	}

	repository.TestUseLowSecurityKDFParameters(t)
	restic.TestDisableCheckPolynomial(t)
	retry.TestFastRetries(t)

	tempdir, err := os.MkdirTemp(rtest.TestTempDir, "restic-test-")
	rtest.OK(t, err)

	env = &testEnvironment{
		base:       tempdir,
		cache:      filepath.Join(tempdir, "cache"),
		repo:       filepath.Join(tempdir, "repo"),
		testdata:   filepath.Join(tempdir, "testdata"),
		mountpoint: filepath.Join(tempdir, "mount"),
	}

	rtest.OK(t, os.MkdirAll(env.mountpoint, 0700))
	rtest.OK(t, os.MkdirAll(env.testdata, 0700))
	rtest.OK(t, os.MkdirAll(env.cache, 0700))
	rtest.OK(t, os.MkdirAll(env.repo, 0700))

	env.gopts = GlobalOptions{
		Repo:     env.repo,
		Quiet:    true,
		CacheDir: env.cache,
		password: rtest.TestPassword,
		extended: make(options.Options),

		// replace this hook with "nil" if listing a filetype more than once is necessary
		backendTestHook: func(r backend.Backend) (backend.Backend, error) { return newOrderedListOnceBackend(r), nil },
		// start with default set of backends
		backends: collectBackends(),
	}

	cleanup = func() {
		if !rtest.TestCleanupTempDirs {
			t.Logf("leaving temporary directory %v used for test", tempdir)
			return
		}
		rtest.RemoveAll(t, tempdir)
	}

	return env, cleanup
}

func testSetupBackupData(t testing.TB, env *testEnvironment) string {
	datafile := filepath.Join("testdata", "backup-data.tar.gz")
	testRunInit(t, env.gopts)
	rtest.SetupTarTestFixture(t, env.testdata, datafile)
	return datafile
}

func listPacks(gopts GlobalOptions, t *testing.T) restic.IDSet {
	var packs restic.IDSet
	err := withTermStatus(t, gopts, func(ctx context.Context, gopts GlobalOptions) error {
		printer := ui.NewProgressPrinter(gopts.JSON, gopts.verbosity, gopts.term)
		ctx, r, unlock, err := openWithReadLock(ctx, gopts, false, printer)
		rtest.OK(t, err)
		defer unlock()

		packs = restic.NewIDSet()

		return r.List(ctx, restic.PackFile, func(id restic.ID, size int64) error {
			packs.Insert(id)
			return nil
		})
	})
	rtest.OK(t, err)
	return packs
}

func listTreePacks(gopts GlobalOptions, t *testing.T) restic.IDSet {
	var treePacks restic.IDSet
	err := withTermStatus(t, gopts, func(ctx context.Context, gopts GlobalOptions) error {
		printer := ui.NewProgressPrinter(gopts.JSON, gopts.verbosity, gopts.term)
		ctx, r, unlock, err := openWithReadLock(ctx, gopts, false, printer)
		rtest.OK(t, err)
		defer unlock()

		rtest.OK(t, r.LoadIndex(ctx, nil))
		treePacks = restic.NewIDSet()
		return r.ListBlobs(ctx, func(pb restic.PackedBlob) {
			if pb.Type == restic.TreeBlob {
				treePacks.Insert(pb.PackID)
			}
		})
	})
	rtest.OK(t, err)
	return treePacks
}

func captureBackend(gopts *GlobalOptions) func() backend.Backend {
	var be backend.Backend
	gopts.backendTestHook = func(r backend.Backend) (backend.Backend, error) {
		be = r
		return r, nil
	}
	return func() backend.Backend {
		return be
	}
}

func removePacks(gopts GlobalOptions, t testing.TB, remove restic.IDSet) {
	be := captureBackend(&gopts)
	err := withTermStatus(t, gopts, func(ctx context.Context, gopts GlobalOptions) error {
		printer := ui.NewProgressPrinter(gopts.JSON, gopts.verbosity, gopts.term)
		ctx, _, unlock, err := openWithExclusiveLock(ctx, gopts, false, printer)
		rtest.OK(t, err)
		defer unlock()

		for id := range remove {
			rtest.OK(t, be().Remove(ctx, backend.Handle{Type: restic.PackFile, Name: id.String()}))
		}
		return nil
	})
	rtest.OK(t, err)
}

func removePacksExcept(gopts GlobalOptions, t testing.TB, keep restic.IDSet, removeTreePacks bool) {
	be := captureBackend(&gopts)
	err := withTermStatus(t, gopts, func(ctx context.Context, gopts GlobalOptions) error {
		printer := ui.NewProgressPrinter(gopts.JSON, gopts.verbosity, gopts.term)
		ctx, r, unlock, err := openWithExclusiveLock(ctx, gopts, false, printer)
		rtest.OK(t, err)
		defer unlock()

		// Get all tree packs
		rtest.OK(t, r.LoadIndex(ctx, nil))

		treePacks := restic.NewIDSet()
		rtest.OK(t, r.ListBlobs(ctx, func(pb restic.PackedBlob) {
			if pb.Type == restic.TreeBlob {
				treePacks.Insert(pb.PackID)
			}
		}))

		// remove all packs containing data blobs
		return r.List(ctx, restic.PackFile, func(id restic.ID, size int64) error {
			if treePacks.Has(id) != removeTreePacks || keep.Has(id) {
				return nil
			}
			return be().Remove(ctx, backend.Handle{Type: restic.PackFile, Name: id.String()})
		})
	})
	rtest.OK(t, err)
}

func includes(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}

	return false
}

func loadSnapshotMap(t testing.TB, gopts GlobalOptions) map[string]struct{} {
	snapshotIDs := testRunList(t, gopts, "snapshots")

	m := make(map[string]struct{})
	for _, id := range snapshotIDs {
		m[id.String()] = struct{}{}
	}

	return m
}

func lastSnapshot(old, new map[string]struct{}) (map[string]struct{}, string) {
	for k := range new {
		if _, ok := old[k]; !ok {
			old[k] = struct{}{}
			return old, k
		}
	}

	return old, ""
}

func testLoadSnapshot(t testing.TB, gopts GlobalOptions, id restic.ID) *data.Snapshot {
	var snapshot *data.Snapshot
	err := withTermStatus(t, gopts, func(ctx context.Context, gopts GlobalOptions) error {
		printer := ui.NewProgressPrinter(gopts.JSON, gopts.verbosity, gopts.term)
		_, repo, unlock, err := openWithReadLock(ctx, gopts, false, printer)
		rtest.OK(t, err)
		defer unlock()

		snapshot, err = data.LoadSnapshot(ctx, repo, id)
		return err
	})
	rtest.OK(t, err)
	return snapshot
}

func appendRandomData(filename string, bytes uint) error {
	f, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE, 0666)
	if err != nil {
		return err
	}

	_, err = f.Seek(0, 2)
	if err != nil {
		return err
	}

	_, err = io.Copy(f, io.LimitReader(rand.Reader, int64(bytes)))
	if err != nil {
		return err
	}

	return f.Close()
}

func testFileSize(filename string, size int64) error {
	fi, err := os.Stat(filename)
	if err != nil {
		return err
	}

	if fi.Size() != size {
		return errors.Fatalf("wrong file size for %v: expected %v, got %v", filename, size, fi.Size())
	}

	return nil
}

func withCaptureStdout(t testing.TB, gopts GlobalOptions, callback func(ctx context.Context, gopts GlobalOptions) error) (*bytes.Buffer, error) {
	buf := bytes.NewBuffer(nil)
	err := withTermStatusRaw(os.Stdin, buf, &logOutputter{t: t}, gopts, callback)
	return buf, err
}

func withTermStatus(t testing.TB, gopts GlobalOptions, callback func(ctx context.Context, gopts GlobalOptions) error) error {
	// stdout and stderr are written to by printer functions etc. That is the written data
	// usually consists of one or multiple lines and therefore can be handled well
	// by t.Log.
	return withTermStatusRaw(os.Stdin, &logOutputter{t: t}, &logOutputter{t: t}, gopts, callback)
}

func withTermStatusRaw(stdin io.ReadCloser, stdout, stderr io.Writer, gopts GlobalOptions, callback func(ctx context.Context, gopts GlobalOptions) error) error {
	ctx, cancel := context.WithCancel(context.TODO())
	var wg sync.WaitGroup

	term := termstatus.New(stdin, stdout, stderr, gopts.Quiet)
	gopts.term = term
	wg.Add(1)
	go func() {
		defer wg.Done()
		term.Run(ctx)
	}()

	defer wg.Wait()
	defer cancel()

	return callback(ctx, gopts)
}
