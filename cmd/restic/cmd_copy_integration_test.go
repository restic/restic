package main

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/restic/restic/internal/data"
	"github.com/restic/restic/internal/global"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
	"github.com/restic/restic/internal/ui"
)

func testRunCopy(t testing.TB, srcGopts global.Options, dstGopts global.Options) {
	gopts := srcGopts
	gopts.Repo = dstGopts.Repo
	gopts.Password = dstGopts.Password
	gopts.InsecureNoPassword = dstGopts.InsecureNoPassword
	copyOpts := CopyOptions{
		SecondaryRepoOptions: global.SecondaryRepoOptions{
			Repo:               srcGopts.Repo,
			Password:           srcGopts.Password,
			InsecureNoPassword: srcGopts.InsecureNoPassword,
		},
	}

	rtest.OK(t, withTermStatus(t, gopts, func(ctx context.Context, gopts global.Options) error {
		return runCopy(context.TODO(), copyOpts, gopts, nil, gopts.Term)
	}))
}

func testRunCopyBatched(t testing.TB, srcGopts global.Options, dstGopts global.Options) {
	gopts := srcGopts
	gopts.Repo = dstGopts.Repo
	gopts.Password = dstGopts.Password
	gopts.InsecureNoPassword = dstGopts.InsecureNoPassword
	copyOpts := CopyOptions{
		SecondaryRepoOptions: global.SecondaryRepoOptions{
			Repo:               srcGopts.Repo,
			Password:           srcGopts.Password,
			InsecureNoPassword: srcGopts.InsecureNoPassword,
		},
		batch: true,
	}

	rtest.OK(t, withTermStatus(t, gopts, func(ctx context.Context, gopts global.Options) error {
		return runCopy(context.TODO(), copyOpts, gopts, nil, gopts.Term)
	}))
}

func TestCopy(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()
	env2, cleanup2 := withTestEnvironment(t)
	defer cleanup2()

	testSetupBackupData(t, env)
	opts := BackupOptions{}
	testRunBackup(t, "", []string{filepath.Join(env.testdata, "0", "0", "9")}, opts, env.gopts)
	testRunBackup(t, "", []string{filepath.Join(env.testdata, "0", "0", "9", "2")}, opts, env.gopts)
	testRunBackup(t, "", []string{filepath.Join(env.testdata, "0", "0", "9", "3")}, opts, env.gopts)
	testRunCheck(t, env.gopts)

	testRunInit(t, env2.gopts)
	testRunCopy(t, env.gopts, env2.gopts)

	snapshotIDs := testListSnapshots(t, env.gopts, 3)
	copiedSnapshotIDs := testListSnapshots(t, env2.gopts, 3)

	// Check that the copies size seems reasonable
	stat := dirStats(t, env.repo)
	stat2 := dirStats(t, env2.repo)
	sizeDiff := int64(stat.size) - int64(stat2.size)
	if sizeDiff < 0 {
		sizeDiff = -sizeDiff
	}
	rtest.Assert(t, sizeDiff < int64(stat.size)/50, "expected less than 2%% size difference: %v vs. %v",
		stat.size, stat2.size)

	// Check integrity of the copy
	testRunCheck(t, env2.gopts)

	// Check that the copied snapshots have the same tree contents as the old ones (= identical tree hash)
	origRestores := make(map[string]struct{})
	for i, snapshotID := range snapshotIDs {
		restoredir := filepath.Join(env.base, fmt.Sprintf("restore%d", i))
		origRestores[restoredir] = struct{}{}
		testRunRestore(t, env.gopts, restoredir, snapshotID.String())
	}
	for i, snapshotID := range copiedSnapshotIDs {
		restoredir := filepath.Join(env2.base, fmt.Sprintf("restore%d", i))
		testRunRestore(t, env2.gopts, restoredir, snapshotID.String())
		foundMatch := false
		for cmpdir := range origRestores {
			diff := directoriesContentsDiff(t, restoredir, cmpdir)
			if diff == "" {
				delete(origRestores, cmpdir)
				foundMatch = true
			}
		}

		rtest.Assert(t, foundMatch, "found no counterpart for snapshot %v", snapshotID)
	}

	rtest.Assert(t, len(origRestores) == 0, "found not copied snapshots")
}

// packfile with size and type
type packInfo struct {
	Type        string
	size        int64
	numberBlobs int
}

// testGetUsedBlobs: call data.FindUsedBlobs for all snapshots in repositpry
func testGetUsedBlobs(t *testing.T, repo restic.Repository) (usedBlobs restic.BlobSet) {
	selectedTrees := make([]restic.ID, 0, 3)
	usedBlobs = restic.NewBlobSet()

	snapshotLister, err := restic.MemorizeList(context.TODO(), repo, restic.SnapshotFile)
	rtest.OK(t, err)
	rtest.OK(t, repo.LoadIndex(context.TODO(), nil))

	// gather all snapshots
	nullFilter := &data.SnapshotFilter{}
	err = nullFilter.FindAll(context.TODO(), snapshotLister, repo, nil, func(_ string, sn *data.Snapshot, err error) error {
		rtest.OK(t, err)
		selectedTrees = append(selectedTrees, *sn.Tree)
		return nil
	})
	rtest.OK(t, err)

	rtest.OK(t, data.FindUsedBlobs(context.TODO(), repo, selectedTrees, usedBlobs, nil))

	return usedBlobs
}

// getPackfileInfo: get packfiles, their length, type and number of blobs in packfile
func getPackfileInfo(t *testing.T, repo restic.Repository) (packfiles map[restic.ID]packInfo) {
	packfiles = make(map[restic.ID]packInfo)

	rtest.OK(t, repo.List(context.TODO(), restic.PackFile, func(id restic.ID, size int64) error {
		blobs, _, err := repo.ListPack(context.TODO(), id, size)
		rtest.OK(t, err)
		rtest.Assert(t, len(blobs) > 0, "a packfile should contain at least one blob")

		Type := ""
		if len(blobs) > 0 {
			Type = blobs[0].Type.String()
		}

		packfiles[id] = packInfo{
			Type:        Type,
			size:        size,
			numberBlobs: len(blobs),
		}

		return nil
	}))

	return packfiles
}

// get various counts from the packfiles in the repository
func getCounts(t *testing.T, repo restic.Repository) (int, int, int) {
	countTreePacks := 0
	countDataPacks := 0
	countBlobs := 0
	for _, item := range getPackfileInfo(t, repo) {
		switch item.Type {
		case "tree":
			countTreePacks++
		case "data":
			countDataPacks++
		}
		countBlobs += item.numberBlobs
	}

	return countTreePacks, countDataPacks, countBlobs
}

func TestCopyBatched(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()
	env3, cleanup3 := withTestEnvironment(t)
	defer cleanup3()

	testSetupBackupData(t, env)
	opts := BackupOptions{}
	testRunBackup(t, "", []string{filepath.Join(env.testdata, "0", "0", "9")}, opts, env.gopts)
	testRunBackup(t, "", []string{filepath.Join(env.testdata, "0", "0", "9", "2")}, opts, env.gopts)
	testRunBackup(t, "", []string{filepath.Join(env.testdata, "0", "0", "9", "3")}, opts, env.gopts)
	testRunCheck(t, env.gopts)

	// batch copy
	testRunInit(t, env3.gopts)
	testRunCopyBatched(t, env.gopts, env3.gopts)

	// check integrity of the copy
	testRunCheck(t, env3.gopts)

	snapshotIDs := testListSnapshots(t, env.gopts, 3)
	copiedSnapshotIDs := testListSnapshots(t, env3.gopts, 3)

	// check that the copied snapshots have the same tree contents as the old ones (= identical tree hash)
	origRestores := make(map[string]struct{})
	for i, snapshotID := range snapshotIDs {
		restoredir := filepath.Join(env.base, fmt.Sprintf("restore%d", i))
		origRestores[restoredir] = struct{}{}
		testRunRestore(t, env.gopts, restoredir, snapshotID.String())
	}

	for i, snapshotID := range copiedSnapshotIDs {
		restoredir := filepath.Join(env3.base, fmt.Sprintf("restore%d", i))
		testRunRestore(t, env3.gopts, restoredir, snapshotID.String())
		foundMatch := false
		for cmpdir := range origRestores {
			diff := directoriesContentsDiff(t, restoredir, cmpdir)
			if diff == "" {
				delete(origRestores, cmpdir)
				foundMatch = true
			}
		}

		rtest.Assert(t, foundMatch, "found no counterpart for snapshot %v", snapshotID)
	}

	rtest.Assert(t, len(origRestores) == 0, "found not copied snapshots")

	// get access to the repositories
	var repo1 restic.Repository
	var unlock1 func()
	var err error
	rtest.OK(t, withTermStatus(t, env.gopts, func(ctx context.Context, gopts global.Options) error {
		printer := ui.NewProgressPrinter(gopts.JSON, gopts.Verbosity, gopts.Term)
		_, repo1, unlock1, err = openWithReadLock(ctx, gopts, false, printer)
		rtest.OK(t, err)
		defer unlock1()
		return err
	}))

	var repo3 restic.Repository
	var unlock3 func()
	rtest.OK(t, withTermStatus(t, env3.gopts, func(ctx context.Context, gopts global.Options) error {
		printer := ui.NewProgressPrinter(gopts.JSON, gopts.Verbosity, gopts.Term)
		_, repo3, unlock3, err = openWithReadLock(ctx, gopts, false, printer)
		rtest.OK(t, err)
		defer unlock3()
		return err
	}))

	usedBlobs1 := testGetUsedBlobs(t, repo1)
	usedBlobs3 := testGetUsedBlobs(t, repo3)
	rtest.Assert(t, len(usedBlobs1) == len(usedBlobs3),
		"used blob length must be identical in both repositories, but is not: (normal) %d <=> (batched) %d",
		len(usedBlobs1), len(usedBlobs3))

	// compare usedBlobs1 <=> usedBlobs3
	good := true
	for bh := range usedBlobs1 {
		if !usedBlobs3.Has(bh) {
			good = false
			break
		}
	}
	rtest.Assert(t, good, "all blobs in both repositories should be equal but they are not")

	_, _, countBlobs1 := getCounts(t, repo1)
	countTreePacks3, countDataPacks3, countBlobs3 := getCounts(t, repo3)

	rtest.Assert(t, countBlobs1 == countBlobs3,
		"expected 1 blob count in boths repos to be equal, but got %d and %d blobs",
		countBlobs1, countBlobs3)

	rtest.Assert(t, countTreePacks3 == 1 && countDataPacks3 == 1,
		"expected 1 data packfile and 1 tree packfile, but got %d trees and %d data packfiles",
		countTreePacks3, countDataPacks3)
}

func TestCopyIncremental(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()
	env2, cleanup2 := withTestEnvironment(t)
	defer cleanup2()

	testSetupBackupData(t, env)
	opts := BackupOptions{}
	testRunBackup(t, "", []string{filepath.Join(env.testdata, "0", "0", "9")}, opts, env.gopts)
	testRunBackup(t, "", []string{filepath.Join(env.testdata, "0", "0", "9", "2")}, opts, env.gopts)
	testRunCheck(t, env.gopts)

	testRunInit(t, env2.gopts)
	testRunCopy(t, env.gopts, env2.gopts)

	testListSnapshots(t, env.gopts, 2)
	testListSnapshots(t, env2.gopts, 2)

	// Check that the copies size seems reasonable
	testRunCheck(t, env2.gopts)

	// check that no snapshots are copied, as there are no new ones
	testRunCopy(t, env.gopts, env2.gopts)
	testRunCheck(t, env2.gopts)
	testListSnapshots(t, env2.gopts, 2)

	// check that only new snapshots are copied
	testRunBackup(t, "", []string{filepath.Join(env.testdata, "0", "0", "9", "3")}, opts, env.gopts)
	testRunCopy(t, env.gopts, env2.gopts)
	testRunCheck(t, env2.gopts)
	testListSnapshots(t, env.gopts, 3)
	testListSnapshots(t, env2.gopts, 3)

	// also test the reverse direction
	testRunCopy(t, env2.gopts, env.gopts)
	testRunCheck(t, env.gopts)
	testListSnapshots(t, env.gopts, 3)
}

func TestCopyUnstableJSON(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()
	env2, cleanup2 := withTestEnvironment(t)
	defer cleanup2()

	// contains a symlink created using `ln -s '../i/'$'\355\246\361''d/samba' broken-symlink`
	datafile := filepath.Join("testdata", "copy-unstable-json.tar.gz")
	rtest.SetupTarTestFixture(t, env.base, datafile)

	testRunInit(t, env2.gopts)
	testRunCopy(t, env.gopts, env2.gopts)
	testRunCheck(t, env2.gopts)
	testListSnapshots(t, env2.gopts, 1)
}

func TestCopyToEmptyPassword(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()
	env2, cleanup2 := withTestEnvironment(t)
	defer cleanup2()
	env2.gopts.Password = ""
	env2.gopts.InsecureNoPassword = true

	testSetupBackupData(t, env)
	testRunBackup(t, "", []string{filepath.Join(env.testdata, "0", "0", "9")}, BackupOptions{}, env.gopts)

	testRunInit(t, env2.gopts)
	testRunCopy(t, env.gopts, env2.gopts)

	testListSnapshots(t, env.gopts, 1)
	testListSnapshots(t, env2.gopts, 1)
	testRunCheck(t, env2.gopts)
}
