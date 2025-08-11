package main

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
)

// packfile with size and type
type packInfo struct {
	Type        string
	size        int64
	numberBlobs int
}

func testRunCopy(t testing.TB, srcGopts GlobalOptions, dstGopts GlobalOptions) {
	gopts := srcGopts
	gopts.Repo = dstGopts.Repo
	gopts.password = dstGopts.password
	gopts.InsecureNoPassword = dstGopts.InsecureNoPassword
	copyOpts := CopyOptions{
		secondaryRepoOptions: secondaryRepoOptions{
			Repo:               srcGopts.Repo,
			password:           srcGopts.password,
			InsecureNoPassword: srcGopts.InsecureNoPassword,
		},
	}

	rtest.OK(t, runCopy(context.TODO(), copyOpts, gopts, nil))
}

func testRunCopyWithStream(t testing.TB, srcGopts GlobalOptions, dstGopts GlobalOptions) {
	gopts := srcGopts
	gopts.Repo = dstGopts.Repo
	gopts.password = dstGopts.password
	gopts.InsecureNoPassword = dstGopts.InsecureNoPassword
	copyOpts := CopyOptions{
		secondaryRepoOptions: secondaryRepoOptions{
			Repo:               srcGopts.Repo,
			password:           srcGopts.password,
			InsecureNoPassword: srcGopts.InsecureNoPassword,
		},
		streamAll: true,
	}

	rtest.OK(t, runCopy(context.TODO(), copyOpts, gopts, nil))
}

func TestCopy(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()
	env2, cleanup2 := withTestEnvironment(t)
	defer cleanup2()
	env3, cleanup3 := withTestEnvironment(t)
	defer cleanup3()

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
	stat := dirStats(env.repo)
	stat2 := dirStats(env2.repo)
	sizeDiff := int64(stat.size) - int64(stat2.size)
	if sizeDiff < 0 {
		sizeDiff = -sizeDiff
	}
	rtest.Assert(t, sizeDiff < int64(stat.size)/50, "expected less than 2%% size difference: %v vs. %v",
		stat.size, stat2.size)

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
			diff := directoriesContentsDiff(restoredir, cmpdir)
			if diff == "" {
				delete(origRestores, cmpdir)
				foundMatch = true
			}
		}

		rtest.Assert(t, foundMatch, "found no counterpart for snapshot %v", snapshotID)
	}

	rtest.Assert(t, len(origRestores) == 0, "found not copied snapshots")

	// streaming copy
	testRunInit(t, env3.gopts)
	testRunCopyWithStream(t, env.gopts, env3.gopts)
	testListSnapshots(t, env3.gopts, 3)

	// compare blobs with non-streaming repository
	_, repo2, unlock2, err := openWithReadLock(context.TODO(), env2.gopts, true)
	rtest.OK(t, err)
	defer unlock2()

	_, repo3, unlock3, err := openWithReadLock(context.TODO(), env3.gopts, true)
	rtest.OK(t, err)
	defer unlock3()

	usedBlobs2 := restic.NewBlobSet()
	testGetUsedBlobs(t, repo2, usedBlobs2)
	usedBlobs3 := restic.NewBlobSet()
	testGetUsedBlobs(t, repo3, usedBlobs3)

	rtest.Assert(t, len(usedBlobs2) == len(usedBlobs3),
		"used blob length must be identical in both repositories, but is not: (normal) %d <=> (streamed) %d",
		len(usedBlobs2), len(usedBlobs3))

	// compare usedBlobs2 <=> usedBlobs3
	good := true
	for bh := range usedBlobs2 {
		if !usedBlobs3.Has(bh) {
			good = false
			break
		}
	}
	rtest.Assert(t, good, "all blobs in both repositories should be equal but they are not")

	packfiles3 := make(map[restic.ID]packInfo)
	testGetPackfiles(t, repo3, packfiles3)
	countTreeBlobs := 0
	countDataBlobs := 0
	countNumberBlobs := 0
	for _, data := range packfiles3 {
		if data.Type == "tree" {
			countTreeBlobs++
		} else if data.Type == "data" {
			countDataBlobs++
		}
		countNumberBlobs += data.numberBlobs
	}

	rtest.Assert(t, countDataBlobs == 1,
		"expected 1 data packfile, but got %d data packfiles", countDataBlobs)
	rtest.Assert(t, len(usedBlobs3) == countNumberBlobs,
		"expected number of used blobs equal to total number of blobs, but used blobs=%d and total=%d",
		len(usedBlobs3), countNumberBlobs)

	packfiles2 := make(map[restic.ID]packInfo)
	testGetPackfiles(t, repo2, packfiles2)
	countTreeBlobs = 0
	countDataBlobs = 0
	countNumberBlobs = 0
	for _, data := range packfiles2 {
		if data.Type == "tree" {
			countTreeBlobs++
		} else if data.Type == "data" {
			countDataBlobs++
		}
		countNumberBlobs += data.numberBlobs
	}

	rtest.Assert(t, countDataBlobs == 3,
		"expected 3 data packfiles, but got %d data packfiles", countDataBlobs)
	rtest.Assert(t, len(usedBlobs2) == countNumberBlobs,
		"expected number of used blobs equal to total number of blobs, but used blobs=%d and total=%d",
		len(usedBlobs2), countNumberBlobs)
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
	env2.gopts.password = ""
	env2.gopts.InsecureNoPassword = true

	testSetupBackupData(t, env)
	testRunBackup(t, "", []string{filepath.Join(env.testdata, "0", "0", "9")}, BackupOptions{}, env.gopts)

	testRunInit(t, env2.gopts)
	testRunCopy(t, env.gopts, env2.gopts)

	testListSnapshots(t, env.gopts, 1)
	testListSnapshots(t, env2.gopts, 1)
	testRunCheck(t, env2.gopts)
}

// testGetUsedBlobs: call FindUsedBlobs for all snapshots in repositpry
func testGetUsedBlobs(t *testing.T, repo restic.Repository, usedBlobs restic.BlobSet) {
	selectedTrees := make([]restic.ID, 0, 10)
	snapshotLister, err := restic.MemorizeList(context.TODO(), repo, restic.SnapshotFile)
	rtest.OK(t, err)
	rtest.OK(t, repo.LoadIndex(context.TODO(), nil))

	nullFilter := &restic.SnapshotFilter{}
	err = nullFilter.FindAll(context.TODO(), snapshotLister, repo, nil, func(_ string, sn *restic.Snapshot, err error) error {
		rtest.OK(t, err)
		selectedTrees = append(selectedTrees, *sn.Tree)
		return nil
	})
	rtest.OK(t, err)
	rtest.Assert(t, len(selectedTrees) == 3, "expected 3 trees, got %d trees instead", len(selectedTrees))

	rtest.OK(t, restic.FindUsedBlobs(context.TODO(), repo, selectedTrees, usedBlobs, nil))
}

// testGetPackfiles: get packfiles, their length, type and number of blobs in packfile
func testGetPackfiles(t *testing.T, repo restic.Repository, packfiles map[restic.ID]packInfo) {
	rtest.OK(t, repo.List(context.TODO(), restic.PackFile, func(id restic.ID, size int64) error {
		blobs, _, err := repo.ListPack(context.TODO(), id, size)
		rtest.OK(t, err)
		Type := "????"
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
}
