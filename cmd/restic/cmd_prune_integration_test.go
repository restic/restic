package main

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/repository"
	rtest "github.com/restic/restic/internal/test"
	"github.com/restic/restic/internal/ui/termstatus"
)

func testRunPrune(t testing.TB, gopts GlobalOptions, opts PruneOptions) {
	oldHook := gopts.backendTestHook
	gopts.backendTestHook = func(r backend.Backend) (backend.Backend, error) { return newListOnceBackend(r), nil }
	defer func() {
		gopts.backendTestHook = oldHook
	}()
	rtest.OK(t, withTermStatus(gopts, func(ctx context.Context, term *termstatus.Terminal) error {
		return runPrune(context.TODO(), opts, gopts, term)
	}))
}

func TestPrune(t *testing.T) {
	testPruneVariants(t, false)
	testPruneVariants(t, true)
}

func testPruneVariants(t *testing.T, unsafeNoSpaceRecovery bool) {
	suffix := ""
	if unsafeNoSpaceRecovery {
		suffix = "-recovery"
	}
	t.Run("0"+suffix, func(t *testing.T) {
		opts := PruneOptions{MaxUnused: "0%", unsafeRecovery: unsafeNoSpaceRecovery}
		checkOpts := CheckOptions{ReadData: true, CheckUnused: !unsafeNoSpaceRecovery}
		testPrune(t, opts, checkOpts)
	})

	t.Run("50"+suffix, func(t *testing.T) {
		opts := PruneOptions{MaxUnused: "50%", unsafeRecovery: unsafeNoSpaceRecovery}
		checkOpts := CheckOptions{ReadData: true}
		testPrune(t, opts, checkOpts)
	})

	t.Run("unlimited"+suffix, func(t *testing.T) {
		opts := PruneOptions{MaxUnused: "unlimited", unsafeRecovery: unsafeNoSpaceRecovery}
		checkOpts := CheckOptions{ReadData: true}
		testPrune(t, opts, checkOpts)
	})

	t.Run("CacheableOnly"+suffix, func(t *testing.T) {
		opts := PruneOptions{MaxUnused: "5%", RepackCacheableOnly: true, unsafeRecovery: unsafeNoSpaceRecovery}
		checkOpts := CheckOptions{ReadData: true}
		testPrune(t, opts, checkOpts)
	})
	t.Run("Small", func(t *testing.T) {
		opts := PruneOptions{MaxUnused: "unlimited", RepackSmall: true}
		checkOpts := CheckOptions{ReadData: true, CheckUnused: true}
		testPrune(t, opts, checkOpts)
	})
}

func createPrunableRepo(t *testing.T, env *testEnvironment) {
	testSetupBackupData(t, env)
	opts := BackupOptions{}

	testRunBackup(t, "", []string{filepath.Join(env.testdata, "0", "0", "9")}, opts, env.gopts)
	firstSnapshot := testListSnapshots(t, env.gopts, 1)[0]

	testRunBackup(t, "", []string{filepath.Join(env.testdata, "0", "0", "9", "2")}, opts, env.gopts)
	testRunBackup(t, "", []string{filepath.Join(env.testdata, "0", "0", "9", "3")}, opts, env.gopts)
	testListSnapshots(t, env.gopts, 3)

	testRunForgetJSON(t, env.gopts)
	testRunForget(t, env.gopts, ForgetOptions{}, firstSnapshot.String())
}

func testRunForgetJSON(t testing.TB, gopts GlobalOptions, args ...string) {
	buf, err := withCaptureStdout(func() error {
		gopts.JSON = true
		opts := ForgetOptions{
			DryRun: true,
			Last:   1,
		}
		pruneOpts := PruneOptions{
			MaxUnused: "5%",
		}
		return withTermStatus(gopts, func(ctx context.Context, term *termstatus.Terminal) error {
			return runForget(context.TODO(), opts, pruneOpts, gopts, term, args)
		})
	})
	rtest.OK(t, err)

	var forgets []*ForgetGroup
	rtest.OK(t, json.Unmarshal(buf.Bytes(), &forgets))

	rtest.Assert(t, len(forgets) == 1,
		"Expected 1 snapshot group, got %v", len(forgets))
	rtest.Assert(t, len(forgets[0].Keep) == 1,
		"Expected 1 snapshot to be kept, got %v", len(forgets[0].Keep))
	rtest.Assert(t, len(forgets[0].Remove) == 2,
		"Expected 2 snapshots to be removed, got %v", len(forgets[0].Remove))
}

func testPrune(t *testing.T, pruneOpts PruneOptions, checkOpts CheckOptions) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	createPrunableRepo(t, env)
	testRunPrune(t, env.gopts, pruneOpts)
	rtest.OK(t, withTermStatus(env.gopts, func(ctx context.Context, term *termstatus.Terminal) error {
		return runCheck(context.TODO(), checkOpts, env.gopts, nil, term)
	}))
}

var pruneDefaultOptions = PruneOptions{MaxUnused: "5%"}

func TestPruneWithDamagedRepository(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	datafile := filepath.Join("testdata", "backup-data.tar.gz")
	testRunInit(t, env.gopts)

	rtest.SetupTarTestFixture(t, env.testdata, datafile)
	opts := BackupOptions{}

	// create and delete snapshot to create unused blobs
	testRunBackup(t, "", []string{filepath.Join(env.testdata, "0", "0", "9", "2")}, opts, env.gopts)
	firstSnapshot := testListSnapshots(t, env.gopts, 1)[0]
	testRunForget(t, env.gopts, ForgetOptions{}, firstSnapshot.String())

	oldPacks := listPacks(env.gopts, t)

	// create new snapshot, but lose all data
	testRunBackup(t, "", []string{filepath.Join(env.testdata, "0", "0", "9", "3")}, opts, env.gopts)
	testListSnapshots(t, env.gopts, 1)
	removePacksExcept(env.gopts, t, oldPacks, false)

	oldHook := env.gopts.backendTestHook
	env.gopts.backendTestHook = func(r backend.Backend) (backend.Backend, error) { return newListOnceBackend(r), nil }
	defer func() {
		env.gopts.backendTestHook = oldHook
	}()
	// prune should fail
	rtest.Assert(t, withTermStatus(env.gopts, func(ctx context.Context, term *termstatus.Terminal) error {
		return runPrune(context.TODO(), pruneDefaultOptions, env.gopts, term)
	}) == repository.ErrPacksMissing,
		"prune should have reported index not complete error")
}

// Test repos for edge cases
func TestEdgeCaseRepos(t *testing.T) {
	opts := CheckOptions{}

	// repo where index is completely missing
	// => check and prune should fail
	t.Run("no-index", func(t *testing.T) {
		testEdgeCaseRepo(t, "repo-index-missing.tar.gz", opts, pruneDefaultOptions, false, false)
	})

	// repo where an existing and used blob is missing from the index
	// => check and prune should fail
	t.Run("index-missing-blob", func(t *testing.T) {
		testEdgeCaseRepo(t, "repo-index-missing-blob.tar.gz", opts, pruneDefaultOptions, false, false)
	})

	// repo where a blob is missing
	// => check and prune should fail
	t.Run("missing-data", func(t *testing.T) {
		testEdgeCaseRepo(t, "repo-data-missing.tar.gz", opts, pruneDefaultOptions, false, false)
	})

	// repo where blobs which are not needed are missing or in invalid pack files
	// => check should fail and prune should repair this
	t.Run("missing-unused-data", func(t *testing.T) {
		testEdgeCaseRepo(t, "repo-unused-data-missing.tar.gz", opts, pruneDefaultOptions, false, true)
	})

	// repo where data exists that is not referenced
	// => check and prune should fully work
	t.Run("unreferenced-data", func(t *testing.T) {
		testEdgeCaseRepo(t, "repo-unreferenced-data.tar.gz", opts, pruneDefaultOptions, true, true)
	})

	// repo where an obsolete index still exists
	// => check and prune should fully work
	t.Run("obsolete-index", func(t *testing.T) {
		testEdgeCaseRepo(t, "repo-obsolete-index.tar.gz", opts, pruneDefaultOptions, true, true)
	})

	// repo which contains mixed (data/tree) packs
	// => check and prune should fully work
	t.Run("mixed-packs", func(t *testing.T) {
		testEdgeCaseRepo(t, "repo-mixed.tar.gz", opts, pruneDefaultOptions, true, true)
	})

	// repo which contains duplicate blobs
	// => checking for unused data should report an error and prune resolves the
	// situation
	opts = CheckOptions{
		ReadData:    true,
		CheckUnused: true,
	}
	t.Run("duplicates", func(t *testing.T) {
		testEdgeCaseRepo(t, "repo-duplicates.tar.gz", opts, pruneDefaultOptions, false, true)
	})
}

func testEdgeCaseRepo(t *testing.T, tarfile string, optionsCheck CheckOptions, optionsPrune PruneOptions, checkOK, pruneOK bool) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	datafile := filepath.Join("testdata", tarfile)
	rtest.SetupTarTestFixture(t, env.base, datafile)

	if checkOK {
		testRunCheck(t, env.gopts)
	} else {
		rtest.Assert(t, withTermStatus(env.gopts, func(ctx context.Context, term *termstatus.Terminal) error {
			return runCheck(context.TODO(), optionsCheck, env.gopts, nil, term)
		}) != nil,
			"check should have reported an error")
	}

	if pruneOK {
		testRunPrune(t, env.gopts, optionsPrune)
		testRunCheck(t, env.gopts)
	} else {
		rtest.Assert(t, withTermStatus(env.gopts, func(ctx context.Context, term *termstatus.Terminal) error {
			return runPrune(context.TODO(), optionsPrune, env.gopts, term)
		}) != nil,
			"prune should have reported an error")
	}
}
