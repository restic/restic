package main

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/restic/restic/internal/global"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
	"github.com/restic/restic/internal/ui/progress"
)

func testRunCatMayFail(t testing.TB, gopts global.Options, args []string) ([]byte, error) {
	t.Helper()
	buf, err := withCaptureStdout(t, gopts, func(ctx context.Context, gopts global.Options) error {
		return runCat(ctx, gopts, args, gopts.Term)
	})

	return buf.Bytes(), err
}

func TestCatMasterkey(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	testSetupBackupData(t, env)
	testRunBackup(t, "", []string{filepath.Join(env.testdata, "0", "0", "9")}, BackupOptions{}, env.gopts)
	testListSnapshots(t, env.gopts, 1)

	stdout, err := testRunCatMayFail(t, env.gopts, []string{"masterkey"})
	rtest.OK(t, err)
	rtest.Assert(t, strings.Contains(string(stdout), "encrypt"), "expected encrypt record")
}

func TestCatConfig(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	testSetupBackupData(t, env)
	testRunBackup(t, "", []string{filepath.Join(env.testdata, "0", "0", "9")}, BackupOptions{}, env.gopts)
	testListSnapshots(t, env.gopts, 1)

	stdout, err := testRunCatMayFail(t, env.gopts, []string{"config"})
	rtest.OK(t, err)

	configUnmarshal := &restic.Config{}
	rtest.OK(t, json.Unmarshal(stdout, configUnmarshal))

	var config restic.Config
	err = withTermStatus(t, env.gopts, func(ctx context.Context, gopts global.Options) error {
		printer := progress.NewTerminalPrinter(gopts.JSON, gopts.Verbosity, gopts.Term)
		_, repo, unlock, err := openWithReadLock(ctx, gopts, false, printer)
		rtest.OK(t, err)
		defer unlock()

		config = repo.Config()
		return err
	})
	rtest.OK(t, err)

	rtest.Assert(t, configUnmarshal.ID == config.ID,
		"expected config IDs to be equal, but %v != %v", configUnmarshal.ID, config.ID)
}

func TestCatWrongType(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	testSetupBackupData(t, env)
	testRunBackup(t, "", []string{filepath.Join(env.testdata, "0", "0", "9")}, BackupOptions{}, env.gopts)
	testListSnapshots(t, env.gopts, 1)

	_, err := testRunCatMayFail(t, env.gopts, []string{"quack"})
	rtest.Assert(t, err != nil && strings.Contains(err.Error(), "invalid type"),
		"expected an 'invalid type' error, got %v", err)
}

type indexPack struct {
	Packs []packJSON `json:"packs"`
}

type packJSON struct {
	ID    restic.ID  `json:"id"`
	Blobs []blobJSON `json:"blobs"`
}

type blobJSON struct {
	ID   restic.ID       `json:"id"`
	Type restic.BlobType `json:"type"`
}

func TestCatIndex(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	testSetupBackupData(t, env)
	testRunBackup(t, "", []string{filepath.Join(env.testdata, "0", "0", "9")}, BackupOptions{}, env.gopts)
	testListSnapshots(t, env.gopts, 1)

	indexIDs := testRunList(t, env.gopts, "index")
	rtest.Assert(t, len(indexIDs) > 0, "expected at least one index file, got none")

	stdout, err := testRunCatMayFail(t, env.gopts, []string{"index", indexIDs[0].String()})
	rtest.OK(t, err)

	// unmarshal "cat index" output into above structs
	indexInfo := &indexPack{}
	rtest.OK(t, json.Unmarshal(stdout, indexInfo))

	// convert ID+Type into a blobSet
	blobHandles := restic.NewBlobSet()
	for _, pack := range indexInfo.Packs {
		for _, item := range pack.Blobs {
			handle := restic.BlobHandle{ID: item.ID, Type: item.Type}
			blobHandles.Insert(handle)
		}
	}
	rtest.Assert(t, len(blobHandles) > 0, "expected a few blobs in the test repo")

	err = withTermStatus(t, env.gopts, func(ctx context.Context, gopts global.Options) error {
		printer := progress.NewTerminalPrinter(gopts.JSON, gopts.Verbosity, gopts.Term)
		_, repo, unlock, err := openWithReadLock(ctx, gopts, false, printer)
		rtest.OK(t, err)
		defer unlock()
		rtest.OK(t, repo.LoadIndex(ctx, printer))

		for handle := range blobHandles {
			_, found := repo.LookupBlobSize(handle)
			rtest.Assert(t, found, "expected to find blob in index, but could not find %v", handle)
		}
		return nil
	})
	rtest.OK(t, err)
}

func TestCatIDs(t *testing.T) {
	const wrongID = "0076fb13c7b340e8455aca6620d50592b0088858c8f18b8b4cc7fbc8abbbaf00"
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	testSetupBackupData(t, env)
	testRunBackup(t, "", []string{filepath.Join(env.testdata, "0", "0", "9")}, BackupOptions{}, env.gopts)
	testListSnapshots(t, env.gopts, 1)

	tests := []struct {
		action  string
		ID      string
		wantErr string
	}{
		{
			action: "pack",
		},
		{
			action:  "pack",
			ID:      wrongID,
			wantErr: "no such file or directory",
		},
		{
			action:  "pack",
			ID:      "QQ76fb13c7b340e8455aca6620d50592b0088858c8f18b8b4cc7fbc8abbbafMM",
			wantErr: "unable to parse ID",
		},
		{
			action: "blob",
		},
		{
			action:  "blob",
			ID:      wrongID,
			wantErr: "blob not found",
		},
		{
			action: "key",
		},
		{
			action:  "key",
			ID:      wrongID,
			wantErr: "no such file or directory",
		},
		{
			action: "lock",
		},
		{
			action: "snapshot",
		},
		{
			action:  "snapshot",
			ID:      wrongID,
			wantErr: "could not find snapshot",
		},
	}

	env.gopts.BackendTestHook = nil
	for _, tc := range tests {
		runName := fmt.Sprintf("cat-%s", tc.action)
		if tc.ID != "" {
			runName = fmt.Sprintf("cat-%s-%s", tc.action, tc.ID)
		}

		t.Run(runName, func(t *testing.T) {
			IDs := testRunList(t, env.gopts, tc.action+"s")
			if len(IDs) == 0 && tc.action == "lock" {
				return
			}
			rtest.Assert(t, len(IDs) > 0, "expected at least one ID for 'list %ss', got none", tc.action)

			expectedError := tc.wantErr
			// correct error string for windows in case of 'file not found'
			if expectedError == "no such file or directory" && runtime.GOOS == "windows" {
				expectedError = "The system cannot find the file specified"
			}

			var err error
			if tc.ID != "" {
				_, err = testRunCatMayFail(t, env.gopts, []string{tc.action, tc.ID})
			} else {
				_, err = testRunCatMayFail(t, env.gopts, []string{tc.action, IDs[0].String()})
			}

			if expectedError == "" {
				rtest.OK(t, err)
			} else {
				rtest.Assert(t, strings.Contains(err.Error(), expectedError),
					"expected error '%v', but got: '%v'", expectedError, err)
			}
		})
	}
}

func TestCatTree(t *testing.T) {
	// I have no idea how to construct the windows version of
	// restic cat tree <snapshot-ID>:/subfolder.
	// The drive letter is getting in the way and filepath.ToSlash() does not fix it properly
	if runtime.GOOS == "windows" {
		return
	}

	const wrongID = "0076fb13c7b340e8455aca6620d50592b0088858c8f18b8b4cc7fbc8abbbaf00"
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	testSetupBackupData(t, env)
	testRunBackup(t, "", []string{filepath.Join(env.testdata, "0", "0", "9")}, BackupOptions{}, env.gopts)
	snapID := testListSnapshots(t, env.gopts, 1)[0]

	out, err := testRunCatMayFail(t, env.gopts, []string{"tree", fmt.Sprintf("%s:%s/0/0", snapID, env.testdata)})
	rtest.OK(t, err)
	rtest.Assert(t, strings.Contains(string(out), `"name":"9","type":"dir"`),
		`expected '"name":"9","type":"dir"' got %v`, string(out))

	_, err = testRunCatMayFail(t, env.gopts, []string{"tree",
		fmt.Sprintf("%s:%s/0/0", wrongID, env.testdata)})
	rtest.Assert(t, err != nil && strings.Contains(err.Error(), "could not find snapshot"),
		"expected 'could not find snapshot', but found %v", err)
}

func TestCatBlob(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	testSetupBackupData(t, env)
	testRunBackup(t, "", []string{filepath.Join(env.testdata, "0", "for_cmd_ls")}, BackupOptions{}, env.gopts)
	testListSnapshots(t, env.gopts, 1)

	err := withTermStatus(t, env.gopts, func(ctx context.Context, gopts global.Options) error {
		printer := progress.NewTerminalPrinter(gopts.JSON, gopts.Verbosity, gopts.Term)
		_, repo, unlock, err := openWithReadLock(ctx, gopts, false, printer)
		rtest.OK(t, err)
		defer unlock()
		rtest.OK(t, repo.LoadIndex(ctx, printer))

		var dataBlob, treeBlob restic.ID
		rtest.OK(t, repo.ListBlobs(ctx, func(pb restic.PackBlob) {
			hand := pb.Handle()
			switch hand.Type {
			case restic.TreeBlob:
				treeBlob = hand.ID
			case restic.DataBlob:
				dataBlob = hand.ID
			default:
				t.Fatalf("\033[31munexpected Type %v for blob %v\033[39m\n", hand.Type, hand.ID)
			}
		}))

		out, err := testRunCatMayFail(t, env.gopts, []string{"blob", treeBlob.String()})
		rtest.OK(t, err)
		rtest.Assert(t, strings.Contains(string(out), `"nodes":[{"name"`),
			`expected "nodes":[{"name" got %v`, string(out))

		_, err = testRunCatMayFail(t, env.gopts, []string{"blob", dataBlob.String()})
		rtest.OK(t, err)

		return nil
	})
	rtest.OK(t, err)
}
