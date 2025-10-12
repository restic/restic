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
	"github.com/restic/restic/internal/ui"
)

func testRunList(t testing.TB, gopts global.Options, tpe string) restic.IDs {
	buf, err := withCaptureStdout(t, gopts, func(ctx context.Context, gopts global.Options) error {
		return runList(ctx, gopts, []string{tpe}, gopts.Term)
	})
	rtest.OK(t, err)
	return parseIDsFromReader(t, buf)
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
	err := withTermStatus(t, gopts, func(ctx context.Context, gopts global.Options) error {
		printer := ui.NewProgressPrinter(gopts.JSON, gopts.Verbosity, gopts.Term)
		_, repo, unlock, err := openWithReadLock(ctx, gopts, false, printer)
		rtest.OK(t, err)
		defer unlock()

		// make sure the index is loaded
		rtest.OK(t, repo.LoadIndex(ctx, nil))

		// get blobs from index
		blobSetFromIndex = restic.NewIDSet()
		rtest.OK(t, repo.ListBlobs(ctx, func(blob restic.PackedBlob) {
			blobSetFromIndex.Insert(blob.ID)
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
	opts := BackupOptions{}

	// first backup
	testRunBackup(t, "", []string{filepath.Join(env.testdata, "0", "0", "9")}, opts, env.gopts)
	testListSnapshots(t, env.gopts, 1)

	// run the `list blobs` command
	resticIDs := testRunList(t, env.gopts, "blobs")

	// convert to set
	testIDSet := restic.NewIDSet(resticIDs...)
	blobSetFromIndex := testListBlobs(t, env.gopts)

	rtest.Assert(t, blobSetFromIndex.Equals(testIDSet), "the set of restic.ID s should be equal")
}
