package main

import (
	"bufio"
	"context"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/restic/restic/internal/global"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
	"github.com/restic/restic/internal/ui/progress"
)

func testRunList(t testing.TB, gopts global.Options, params ...string) restic.IDs {
	t.Helper()
	buf, err := withCaptureStdout(t, gopts, func(ctx context.Context, gopts global.Options) error {
		return runList(ctx, gopts, params, gopts.Term, "")
	})
	rtest.OK(t, err)
	return parseIDsFromReader(t, buf)
}

func testRunListMayFail(t testing.TB, gopts global.Options, params []string) error {
	t.Helper()
	_, err := withCaptureStdout(t, gopts, func(ctx context.Context, gopts global.Options) error {
		return runList(ctx, gopts, params, gopts.Term, "")
	})
	return err
}

func parseIDsFromReader(t testing.TB, rd io.Reader) restic.IDs {
	t.Helper()
	IDs := restic.IDs{}
	sc := bufio.NewScanner(rd)

	for sc.Scan() {
		if len(sc.Text()) == 64 {
			id, err := restic.ParseID(sc.Text())
			if err != nil {
				t.Logf("parse id %v: %v", sc.Text(), err)
				continue
			}
			IDs = append(IDs, id)
		} else {
			// 'list blobs' is different because it lists the blobs together with the blob type
			// e.g. "tree ac08ce34ba4f8123618661bef2425f7028ffb9ac740578a3ee88684d2523fee8"
			parts := strings.Split(sc.Text(), " ")
			id, err := restic.ParseID(parts[len(parts)-1])
			if err != nil {
				t.Logf("parse id %v: %v", sc.Text(), err)
				continue
			}
			IDs = append(IDs, id)
		}
	}

	return IDs
}

func testListSnapshots(t testing.TB, gopts global.Options, expected int) restic.IDs {
	t.Helper()
	snapshotIDs := testRunList(t, gopts, "snapshots")
	rtest.Assert(t, len(snapshotIDs) == expected, "expected %v snapshot, got %v", expected, snapshotIDs)
	return snapshotIDs
}

// extract blob set from repository index
func testListBlobs(t testing.TB, gopts global.Options) (blobSetFromIndex restic.IDSet) {
	t.Helper()
	err := withTermStatus(t, gopts, func(ctx context.Context, gopts global.Options) error {
		printer := progress.NewTerminalPrinter(gopts.JSON, gopts.Verbosity, gopts.Term)
		_, repo, unlock, err := openWithReadLock(ctx, gopts, false, printer)
		rtest.OK(t, err)
		defer unlock()

		// make sure the index is loaded
		rtest.OK(t, repo.LoadIndex(ctx, restic.NoopTerminalCounterFactory))

		// get blobs from index
		blobSetFromIndex = restic.NewIDSet()
		rtest.OK(t, repo.ListBlobs(ctx, func(blob restic.PackBlob) {
			blobSetFromIndex.Insert(blob.Handle().ID)
		}))
		return nil
	})
	rtest.OK(t, err)
	return blobSetFromIndex
}

func TestListBlobs(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()
	testSetupBackupData(t, env)

	testRunBackup(t, "", []string{filepath.Join(env.testdata, "0", "0", "9")}, BackupOptions{}, env.gopts)
	testListSnapshots(t, env.gopts, 1)

	// run the `list blobs` command and convert to set
	testIDSet := restic.NewIDSet(testRunList(t, env.gopts, "blobs")...)
	blobSetFromIndex := testListBlobs(t, env.gopts)

	rtest.Assert(t, blobSetFromIndex.Equals(testIDSet), "the set of restic.IDs should be equal")
}

func TestPackfileListWithSnapshot(t *testing.T) {
	// setup
	env, cleanup := withTestEnvironment(t)
	defer cleanup()
	testSetupBackupData(t, env)

	// 3 backups, single file each
	opts := BackupOptions{}
	testRunBackup(t, env.testdata, []string{filepath.Join(env.testdata, "0", "0", "9", "40")}, opts, env.gopts)
	testRunBackup(t, env.testdata, []string{filepath.Join(env.testdata, "0", "0", "9", "41")}, opts, env.gopts)
	testRunBackup(t, env.testdata, []string{filepath.Join(env.testdata, "0", "0", "9", "42")}, opts, env.gopts)
	testListSnapshots(t, env.gopts, 3)

	// run packfilelist
	packfiles := testRunList(t, env.gopts, "packs")
	rtest.Assert(t, len(packfiles) == 6, "expected 6 packfiles in repository, got %d", len(packfiles))

	packfiles = testRunList(t, env.gopts, "packs", "latest")
	rtest.Assert(t, len(packfiles) == 2, "expected 2 packfiles in snapshot, got %d", len(packfiles))
}

func TestListModes(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	testSetupBackupData(t, env)
	testRunBackup(t, "", []string{filepath.Join(env.testdata, "0", "0", "9")}, BackupOptions{}, env.gopts)
	testListSnapshots(t, env.gopts, 1)

	tests := []struct {
		name    string
		action  []string
		wantErr string // substring of expected error, "" for no error
	}{
		{
			name:   "index",
			action: []string{"index"},
		},
		{
			name:   "locks",
			action: []string{"locks"},
		},
		{
			name:    "wrong list option",
			action:  []string{"quark"},
			wantErr: "invalid type",
		},
		{
			name:    "too many parameters",
			action:  []string{"index", "invalid"},
			wantErr: "too many parameters",
		},
		{
			name:    "unknown pack",
			action:  []string{"packs", "deadbeef"},
			wantErr: "failed to find snapshot",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := testRunListMayFail(t, env.gopts, tc.action)
			if err != nil {
				rtest.Assert(t, strings.Contains(err.Error(), tc.wantErr),
					"expected %q, got %v", tc.wantErr, err)
			} else if tc.wantErr != "" {
				t.Fatalf("expected error '%s', but got none", tc.wantErr)
			}
		})
	}
}
