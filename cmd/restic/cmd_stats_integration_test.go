package main

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/restic/restic/internal/global"
	rtest "github.com/restic/restic/internal/test"
)

func testRunStats(t testing.TB, wantJSON bool, opts StatsOptions, gopts global.Options, args []string) []byte {
	buf, err := withCaptureStdout(t, gopts, func(ctx context.Context, gopts global.Options) error {
		gopts.JSON = wantJSON

		return runStats(ctx, opts, gopts, args, gopts.Term)
	})
	rtest.OK(t, err)
	return buf.Bytes()
}

func TestStatsModeInfo(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	testSetupBackupData(t, env)
	opts := BackupOptions{}
	// first backup of subtree 0/0/9
	testRunBackup(t, env.testdata, []string{filepath.Join(env.testdata, "0", "0", "9")}, opts, env.gopts)

	optsStats := StatsOptions{countMode: "info"}
	output := testRunStats(t, true, optsStats, env.gopts, nil)

	info := infoStats{}
	rtest.OK(t, json.Unmarshal(output, &info))

	rtest.Equals(t, 1, info.General.SnapshotsCount)
	rtest.Equals(t, 1, info.General.TreeCount)
	rtest.Equals(t, 69, info.UniqueFiles.UniqueFilesByContents)

	rtest.Equals(t, 69, info.Blobs.DataBlobs)
	rtest.Equals(t, 0, info.Blobs.UnusedBlobs)
	rtest.Equals(t, 0, info.Blobs.DuplicateBlobRefs)

	rtest.Equals(t, 69, info.Trees.CountAllFiles)
	rtest.Equals(t, 2, info.Packfiles.TotalPackFiles)

	// second backup of the whole lot
	testRunBackup(t, env.testdata, []string{filepath.Join(env.testdata, "0")}, opts, env.gopts)
	output = testRunStats(t, true, optsStats, env.gopts, nil)
	rtest.OK(t, json.Unmarshal(output, &info))

	rtest.Equals(t, 2, info.General.SnapshotsCount)
	rtest.Equals(t, 2, info.General.TreeCount)
	// windows counts harlinks differently
	//rtest.Equals(t, 69+5, info.UniqueFiles.UniqueFilesByContents)

	rtest.Equals(t, 73, info.Blobs.DataBlobs)
	rtest.Equals(t, 0, info.Blobs.UnusedBlobs)
	rtest.Equals(t, 0, info.Blobs.DuplicateBlobRefs)

	rtest.Equals(t, 2, info.Packfiles.CountTreePackfiles)
	rtest.Equals(t, 2, info.Packfiles.CountDataPackfiles)
	rtest.Equals(t, 4, info.Packfiles.CountFullPackfiles)
}
