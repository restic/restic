package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/restic/restic/internal/data"
	"github.com/restic/restic/internal/filter"
	"github.com/restic/restic/internal/global"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
)

func testRunSnapshots(t testing.TB, gopts global.Options) (newest *Snapshot, snapmap map[restic.ID]Snapshot) {
	buf, err := withCaptureStdout(t, gopts, func(ctx context.Context, gopts global.Options) error {
		gopts.JSON = true

		opts := SnapshotOptions{}
		return runSnapshots(ctx, opts, gopts, []string{}, gopts.Term)
	})
	rtest.OK(t, err)

	snapshots := []Snapshot{}
	rtest.OK(t, json.Unmarshal(buf.Bytes(), &snapshots))

	snapmap = make(map[restic.ID]Snapshot, len(snapshots))
	for _, sn := range snapshots {
		snapmap[*sn.ID] = sn
		if newest == nil || sn.Time.After(newest.Time) {
			newest = &sn
		}
	}
	return
}

func snapshotsGroupTestData(t *testing.T, env *testEnvironment, keepPath bool) string {
	testSetupBackupData(t, env)
	// two backups on the same host but with different paths
	opts := BackupOptions{Host: "testhost", TimeStamp: time.Now().Format(time.DateTime)}
	testRunBackup(t, filepath.Dir(env.testdata), []string{"testdata"}, opts, env.gopts)
	// Use later timestamp for second backup
	opts.TimeStamp = time.Now().Add(time.Second).Format(time.DateTime)
	snapshotsIDs := loadSnapshotMap(t, env.gopts)

	targets := []string{"testdata/0"}
	if keepPath {
		targets = []string{"testdata"}
	}
	testRunBackup(t, filepath.Dir(env.testdata), targets, opts, env.gopts)
	_, secondSnapshotID := lastSnapshot(snapshotsIDs, loadSnapshotMap(t, env.gopts))

	return secondSnapshotID
}

func TestSnapshotsGroupByAndLatest(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	secondSnapshotID := snapshotsGroupTestData(t, env, false)
	buf, err := withCaptureStdout(t, env.gopts, func(ctx context.Context, gopts global.Options) error {
		gopts.JSON = true
		// only group by host but not path
		opts := SnapshotOptions{GroupBy: data.SnapshotGroupByOptions{Host: true}, Latest: 1}
		return runSnapshots(ctx, opts, gopts, []string{}, gopts.Term)
	})
	rtest.OK(t, err)
	snapshots := []SnapshotGroup{}
	rtest.OK(t, json.Unmarshal(buf.Bytes(), &snapshots))
	rtest.Assert(t, len(snapshots) == 1, "expected only one snapshot group, got %d", len(snapshots))
	rtest.Assert(t, snapshots[0].GroupKey.Hostname == "testhost", "expected group_key.hostname to be set to testhost, got %s", snapshots[0].GroupKey.Hostname)
	rtest.Assert(t, snapshots[0].GroupKey.Paths == nil, "expected group_key.paths to be set to nil, got %s", snapshots[0].GroupKey.Paths)
	rtest.Assert(t, snapshots[0].GroupKey.Tags == nil, "expected group_key.tags to be set to nil, got %s", snapshots[0].GroupKey.Tags)
	rtest.Assert(t, len(snapshots[0].Snapshots) == 1, "expected only one latest snapshot, got %d", len(snapshots[0].Snapshots))
	rtest.Equals(t, snapshots[0].Snapshots[0].ID.String(), secondSnapshotID, "unexpected snapshot ID")
}

func TestSnapshotsLatest(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	secondSnapshotID := snapshotsGroupTestData(t, env, true)

	buf, err := withCaptureStdout(t, env.gopts, func(ctx context.Context, gopts global.Options) error {
		gopts.JSON = true
		opts := SnapshotOptions{Latest: 1}
		return runSnapshots(ctx, opts, gopts, []string{}, gopts.Term)
	})
	rtest.OK(t, err)
	snapshots := []Snapshot{}
	rtest.OK(t, json.Unmarshal(buf.Bytes(), &snapshots))
	rtest.Assert(t, len(snapshots) == 1, "expected only one snapshot, got %d", len(snapshots))
	rtest.Equals(t, snapshots[0].ID.String(), secondSnapshotID, "unexpected snapshot ID")
}

func TestSnapshotsExcludeItems(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	excludePatterns := []string{"*.c", "*.so", "*.h"}
	// Create an exclude file with above patterns
	patternsFile := env.base + "/patternsFile"
	fileErr := os.WriteFile(patternsFile, []byte(strings.Join(excludePatterns, "\n")), 0600)
	rtest.OK(t, fileErr)

	testSetupBackupData(t, env)
	opts := BackupOptions{
		ExcludePatternOptions: filter.ExcludePatternOptions{
			Excludes:     []string{"quatsch"},
			ExcludeFiles: []string{patternsFile},
		},
	}

	testRunBackup(t, "", []string{env.testdata}, opts, env.gopts)

	buf, err := withCaptureStdout(t, env.gopts, func(ctx context.Context, gopts global.Options) error {
		gopts.JSON = true
		opts := SnapshotOptions{Latest: 1}
		return runSnapshots(ctx, opts, gopts, []string{}, gopts.Term)
	})
	rtest.OK(t, err)

	snapshots := []Snapshot{}
	rtest.OK(t, json.Unmarshal(buf.Bytes(), &snapshots))
	rtest.Assert(t, len(snapshots) == 1, "expected only one snapshot, got %d", len(snapshots))
	rtest.Assert(t, len(snapshots[0].Excludes) == 4, "expected 4 exclude items, got %d", len(snapshots[0].Excludes))
	rtest.Equals(t, "*.h", snapshots[0].Excludes[3])
}
