package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/data"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/global"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
	"github.com/restic/restic/internal/ui"
)

func TestCheckRestoreNoLock(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	datafile := filepath.Join("testdata", "small-repo.tar.gz")
	rtest.SetupTarTestFixture(t, env.base, datafile)

	err := filepath.Walk(env.repo, func(p string, fi os.FileInfo, e error) error {
		if e != nil {
			return e
		}
		return os.Chmod(p, fi.Mode() & ^(os.FileMode(0222)))
	})
	rtest.OK(t, err)

	env.gopts.NoLock = true

	testRunCheck(t, env.gopts)

	snapshotIDs := testListSnapshots(t, env.gopts, 4)
	testRunRestore(t, env.gopts, filepath.Join(env.base, "restore"), snapshotIDs[0].String())
}

// a listOnceBackend only allows listing once per filetype
// listing filetypes more than once may cause problems with eventually consistent
// backends (like e.g. Amazon S3) as the second listing may be inconsistent to what
// is expected by the first listing + some operations.
type listOnceBackend struct {
	backend.Backend
	listedFileType map[restic.FileType]bool
	strictOrder    bool
}

func newListOnceBackend(be backend.Backend) *listOnceBackend {
	return &listOnceBackend{
		Backend:        be,
		listedFileType: make(map[restic.FileType]bool),
		strictOrder:    false,
	}
}

func newOrderedListOnceBackend(be backend.Backend) *listOnceBackend {
	return &listOnceBackend{
		Backend:        be,
		listedFileType: make(map[restic.FileType]bool),
		strictOrder:    true,
	}
}

func (be *listOnceBackend) List(ctx context.Context, t restic.FileType, fn func(backend.FileInfo) error) error {
	if t != restic.LockFile && be.listedFileType[t] {
		return errors.Errorf("tried listing type %v the second time", t)
	}
	if be.strictOrder && t == restic.SnapshotFile && be.listedFileType[restic.IndexFile] {
		return errors.Errorf("tried listing type snapshots after index")
	}
	be.listedFileType[t] = true
	return be.Backend.List(ctx, t, fn)
}

func TestListOnce(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	env.gopts.BackendTestHook = func(r backend.Backend) (backend.Backend, error) {
		return newOrderedListOnceBackend(r), nil
	}
	pruneOpts := PruneOptions{MaxUnused: "0"}
	checkOpts := CheckOptions{ReadData: true, CheckUnused: true}

	createPrunableRepo(t, env)
	testRunPrune(t, env.gopts, pruneOpts)
	rtest.OK(t, withTermStatus(t, env.gopts, func(ctx context.Context, gopts global.Options) error {
		_, err := runCheck(context.TODO(), checkOpts, gopts, nil, gopts.Term)
		return err
	}))
	rtest.OK(t, withTermStatus(t, env.gopts, func(ctx context.Context, gopts global.Options) error {
		return runRebuildIndex(context.TODO(), RepairIndexOptions{}, gopts, gopts.Term)
	}))
	rtest.OK(t, withTermStatus(t, env.gopts, func(ctx context.Context, gopts global.Options) error {
		return runRebuildIndex(context.TODO(), RepairIndexOptions{ReadAllPacks: true}, gopts, gopts.Term)
	}))
}

type writeToOnly struct {
	rd io.Reader
}

func (r *writeToOnly) Read(_ []byte) (n int, err error) {
	return 0, fmt.Errorf("should have called WriteTo instead")
}

func (r *writeToOnly) WriteTo(w io.Writer) (int64, error) {
	return io.Copy(w, r.rd)
}

type onlyLoadWithWriteToBackend struct {
	backend.Backend
}

func (be *onlyLoadWithWriteToBackend) Load(ctx context.Context, h backend.Handle,
	length int, offset int64, fn func(rd io.Reader) error) error {

	return be.Backend.Load(ctx, h, length, offset, func(rd io.Reader) error {
		return fn(&writeToOnly{rd: rd})
	})
}

func TestBackendLoadWriteTo(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	// setup backend which only works if it's WriteTo method is correctly propagated upwards
	env.gopts.BackendInnerTestHook = func(r backend.Backend) (backend.Backend, error) {
		return &onlyLoadWithWriteToBackend{Backend: r}, nil
	}

	testSetupBackupData(t, env)

	// add some data, but make sure that it isn't cached during upload
	opts := BackupOptions{}
	env.gopts.NoCache = true
	testRunBackup(t, "", []string{filepath.Join(env.testdata, "0", "0", "9")}, opts, env.gopts)

	// loading snapshots must still work
	env.gopts.NoCache = false
	testListSnapshots(t, env.gopts, 1)
}

func TestFindListOnce(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	env.gopts.BackendTestHook = func(r backend.Backend) (backend.Backend, error) {
		return newOrderedListOnceBackend(r), nil
	}

	testSetupBackupData(t, env)
	opts := BackupOptions{}

	testRunBackup(t, "", []string{filepath.Join(env.testdata, "0", "0", "9")}, opts, env.gopts)
	testRunBackup(t, "", []string{filepath.Join(env.testdata, "0", "0", "9", "2")}, opts, env.gopts)
	secondSnapshot := testListSnapshots(t, env.gopts, 2)
	testRunBackup(t, "", []string{filepath.Join(env.testdata, "0", "0", "9", "3")}, opts, env.gopts)
	thirdSnapshot := restic.NewIDSet(testListSnapshots(t, env.gopts, 3)...)

	var snapshotIDs restic.IDSet
	rtest.OK(t, withTermStatus(t, env.gopts, func(ctx context.Context, gopts global.Options) error {
		printer := ui.NewProgressPrinter(gopts.JSON, gopts.Verbosity, gopts.Term)
		ctx, repo, unlock, err := openWithReadLock(ctx, gopts, false, printer)
		rtest.OK(t, err)
		defer unlock()

		snapshotIDs = restic.NewIDSet()
		// specify the two oldest snapshots explicitly and use "latest" to reference the newest one
		for sn := range FindFilteredSnapshots(ctx, repo, repo, &data.SnapshotFilter{}, []string{
			secondSnapshot[0].String(),
			secondSnapshot[1].String()[:8],
			"latest",
		}, printer) {
			snapshotIDs.Insert(*sn.ID())
		}
		return nil
	}))

	// the snapshots can only be listed once, if both lists match then the there has been only a single List() call
	rtest.Equals(t, thirdSnapshot, snapshotIDs)
}

type failConfigOnceBackend struct {
	backend.Backend
	failedOnce bool
}

func (be *failConfigOnceBackend) Load(ctx context.Context, h backend.Handle,
	length int, offset int64, fn func(rd io.Reader) error) error {

	if !be.failedOnce && h.Type == restic.ConfigFile {
		be.failedOnce = true
		return fmt.Errorf("oops")
	}
	return be.Backend.Load(ctx, h, length, offset, fn)
}

func (be *failConfigOnceBackend) Stat(ctx context.Context, h backend.Handle) (backend.FileInfo, error) {
	if !be.failedOnce && h.Type == restic.ConfigFile {
		be.failedOnce = true
		return backend.FileInfo{}, fmt.Errorf("oops")
	}
	return be.Backend.Stat(ctx, h)
}

func TestBackendRetryConfig(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	var wrappedBackend *failConfigOnceBackend
	// cause config loading to fail once
	env.gopts.BackendInnerTestHook = func(r backend.Backend) (backend.Backend, error) {
		wrappedBackend = &failConfigOnceBackend{Backend: r}
		return wrappedBackend, nil
	}

	testSetupBackupData(t, env)
	rtest.Assert(t, wrappedBackend != nil, "backend not wrapped on init")
	rtest.Assert(t, wrappedBackend != nil && wrappedBackend.failedOnce, "config loading was not retried on init")
	wrappedBackend = nil

	testRunBackup(t, "", []string{filepath.Join(env.testdata, "0", "0", "9")}, BackupOptions{}, env.gopts)
	rtest.Assert(t, wrappedBackend != nil, "backend not wrapped on backup")
	rtest.Assert(t, wrappedBackend != nil && wrappedBackend.failedOnce, "config loading was not retried on init")
}
