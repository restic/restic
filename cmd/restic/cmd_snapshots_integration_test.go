package main

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/restic/restic/internal/data"
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

func TestSnapshotsGroupByAndLatest(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	testSetupBackupData(t, env)
	// two backups on the same host but with different paths
	opts := BackupOptions{Host: "testhost", TimeStamp: time.Now().Format(time.DateTime)}
	testRunBackup(t, filepath.Dir(env.testdata), []string{"testdata"}, opts, env.gopts)
	// Use later timestamp for second backup
	opts.TimeStamp = time.Now().Add(time.Second).Format(time.DateTime)
	snapshotsIDs := loadSnapshotMap(t, env.gopts)
	testRunBackup(t, filepath.Dir(env.testdata), []string{"testdata/0"}, opts, env.gopts)
	_, secondSnapshotID := lastSnapshot(snapshotsIDs, loadSnapshotMap(t, env.gopts))

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

func testRunSnapshotsOpts(t testing.TB, gopts global.Options, opts SnapshotOptions) (newest *Snapshot, snapmap map[restic.ID]Snapshot) {
	buf, err := withCaptureStdout(t, gopts, func(ctx context.Context, gopts global.Options) error {
		gopts.JSON = true

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

func testSetDuration(t *testing.T, duration string) (result data.DurationTime) {
	rtest.OK(t, (&result).Set(duration))
	return result
}

func TestSnapshotTimeFilter(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	startTime := time.Now()
	testSetupBackupData(t, env)
	opts := BackupOptions{}

	// create 5 backups which will receive changed backup timestamps
	for i := 0; i < 5; i++ {
		testRunBackup(t, env.testdata+"/0", []string{"0/9"}, opts, env.gopts)
	}
	snapshotIDs := testListSnapshots(t, env.gopts, 5)

	// prepare to set new times for all snapshots
	timeReference := testSetDuration(t, startTime.Format(time.DateTime))
	offsets := []data.DurationTime{
		testSetDuration(t, "0h"),
		testSetDuration(t, "2d"),
		testSetDuration(t, "1m"),
		testSetDuration(t, "2m"),
		testSetDuration(t, "6m"),
	}

	// convert Durations to timestamps
	newTimes := make([]data.DurationTime, len(offsets))
	for i, offset := range offsets {
		newTimes[i] = timeReference.AddOffset(offset)
	}

	// rewrite the backup timestamps for all given backups
	for i, snapID := range snapshotIDs {
		temp1 := newTimes[i].GetTime()
		rewriteOpts := RewriteOptions{
			Metadata: snapshotMetadataArgs{
				Time: temp1.Format(time.DateTime),
			},
			Forget: true,
		}
		rtest.OK(t, withTermStatus(t, env.gopts, func(ctx context.Context, gopts global.Options) error {
			return runRewrite(ctx, rewriteOpts, gopts, []string{snapID.String()}, gopts.Term)
		}))
	}
	testListSnapshots(t, env.gopts, 5)

	// extract all snapshots and their backup timestamps into
	referenceT := testSetDuration(t, startTime.Format(time.DateTime))
	latest := testSetDuration(t, "latest")
	optsSnap := SnapshotOptions{
		SnapshotFilter: data.SnapshotFilter{
			OlderThan:  latest,
			RelativeTo: referenceT,
		},
	}
	_, snapmap := testRunSnapshotsOpts(t, env.gopts, optsSnap)
	rtest.Assert(t, len(snapmap) == 5, "there should be 5 snapshots in all, but there are %d of them", len(snapmap))

	// build 'durationMap' so we can find the snapID which belongs to a particular timestamp
	durationMap := make(map[int]restic.ID, len(snapmap))
	for ID, sn := range snapmap {
		for i, backupTime := range newTimes {
			if backupTime.GetTime().Equal(sn.Time) {
				durationMap[i] = ID
				break
			}
		}
	}
	rtest.Assert(t, len(durationMap) == 5, "there should be 5 entries in 'durationMap', but there are %d of them", len(durationMap))

	olderThan := offsets[2]
	newerThan := offsets[3]
	shiftOlder := timeReference.AddOffset(olderThan)
	shiftNewer := timeReference.AddOffset(newerThan)

	var dtOlder data.DurationTime
	var dtNewer data.DurationTime
	rtest.OK(t, dtOlder.Set(durationMap[2].String()))
	rtest.OK(t, dtNewer.Set(durationMap[3].String()))

	// ================== OlderThan ==================
	// the 3 variables here represent the same timestamp in the possible 3 formats
	// a Duration, a timestamp (as calculated from an Duration offset), a snapID from a rewritten backup
	for _, dt := range []data.DurationTime{olderThan, shiftOlder, dtOlder} {
		optsSnap = SnapshotOptions{SnapshotFilter: data.SnapshotFilter{
			OlderThan:  dt,
			RelativeTo: referenceT,
		}}
		_, snapmap := testRunSnapshotsOpts(t, env.gopts, optsSnap)
		// 1m, 2m, 6m
		rtest.Assert(t, len(snapmap) == 3, "there should be 3 snapshots older than 1 month, but there are %d of them", len(snapmap))
	}

	for _, dt := range []data.DurationTime{olderThan, shiftOlder, dtOlder} {
		optsSnap = SnapshotOptions{SnapshotFilter: data.SnapshotFilter{
			OlderThan: dt,
			// relative-to latest
		}}
		_, snapmap := testRunSnapshotsOpts(t, env.gopts, optsSnap)
		// 1m, 2m, 6m
		rtest.Assert(t, len(snapmap) == 3, "there should be 3 snapshots older than 1 month, but there are %d of them", len(snapmap))
	}

	// ================== NewerThan ==================
	for _, dt := range []data.DurationTime{newerThan, shiftNewer, dtNewer} {
		optsSnap = SnapshotOptions{SnapshotFilter: data.SnapshotFilter{
			NewerThan:  dt,
			RelativeTo: referenceT,
		}}
		_, snapmap := testRunSnapshotsOpts(t, env.gopts, optsSnap)

		// 2m, 1m, 2d, 0h
		rtest.Assert(t, len(snapmap) == 4, "there should be 4 snapshots newer than 1 month, but there are %d of them", len(snapmap))
	}

	for _, dt := range []data.DurationTime{shiftNewer, dtNewer, newerThan} {
		optsSnap = SnapshotOptions{SnapshotFilter: data.SnapshotFilter{
			NewerThan: dt,
			// relative-to latest
		}}
		_, snapmap := testRunSnapshotsOpts(t, env.gopts, optsSnap)

		// 2m, 1m, 2d, 0h
		rtest.Assert(t, len(snapmap) == 4, "there should be 4 snapshots newer than 1 month, but there are %d of them", len(snapmap))
	}

	// ================== both time filters ==================
	type both struct {
		NewerThan data.DurationTime
		OlderThan data.DurationTime
	}

	for _, dt := range []both{{newerThan, olderThan}, {shiftNewer, shiftOlder}, {dtNewer, dtOlder}} {
		optsSnap = SnapshotOptions{SnapshotFilter: data.SnapshotFilter{
			NewerThan:  dt.NewerThan,
			OlderThan:  dt.OlderThan,
			RelativeTo: referenceT,
		}}
		_, snapmap = testRunSnapshotsOpts(t, env.gopts, optsSnap)
		// 1m 2m
		rtest.Assert(t, len(snapmap) == 2, "there should be 2 snapshots between 1 and 2 months, but there are %d of them", len(snapmap))
	}

	for _, dt := range []both{{newerThan, olderThan}, {shiftNewer, shiftOlder}, {dtNewer, dtOlder}} {
		optsSnap = SnapshotOptions{SnapshotFilter: data.SnapshotFilter{
			NewerThan: dt.NewerThan,
			OlderThan: dt.OlderThan,
			// relative-to latest
		}}
		_, snapmap = testRunSnapshotsOpts(t, env.gopts, optsSnap)
		// 1m 2m
		rtest.Assert(t, len(snapmap) == 2, "there should be 2 snapshots between 1 and 2 months, but there are %d of them", len(snapmap))
	}
}
