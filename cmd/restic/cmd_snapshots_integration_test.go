package main

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"
	"slices"
	"maps"

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
	if err != nil && err.Error()[:25] == "no snapshot matched given" {
		err = nil
	}
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
	for range 5 {
		testRunBackup(t, filepath.Join(env.testdata, "0"), []string{filepath.Join("0", "9")}, opts, env.gopts)
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
	referenceTime := testSetDuration(t, startTime.Format(time.DateTime))
	latest := testSetDuration(t, "latest")
	optsSnap := SnapshotOptions{
		SnapshotFilter: data.SnapshotFilter{
			UpperTimeLimit: latest,
			RelativeTo:     referenceTime,
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

	lowerTimeLimit := offsets[3] // 2m
	upperTimeLimit := offsets[2] // 1m
	shiftLower := timeReference.AddOffset(lowerTimeLimit)
	shiftUpper := timeReference.AddOffset(upperTimeLimit)

	var dtUpper data.DurationTime
	var dtLower data.DurationTime
	rtest.OK(t, dtUpper.Set(durationMap[2].String()))
	rtest.OK(t, dtLower.Set(durationMap[3].String()))

	// ================== UpperTimeLimit ==================
	// the 3 variables here represent the same timestamp in the possible 3 formats
	// a Duration, a timestamp (as calculated from an Duration offset), a snapID from a rewritten backup
	for _, dt := range []data.DurationTime{upperTimeLimit, shiftUpper, dtUpper} {
		optsSnap = SnapshotOptions{SnapshotFilter: data.SnapshotFilter{
			UpperTimeLimit: dt,
			RelativeTo:     referenceTime,
		}}
		_, snapmap := testRunSnapshotsOpts(t, env.gopts, optsSnap)
		// 1m, 2m, 6m
		rtest.Assert(t, len(snapmap) == 3, "there should be 3 snapshots older than 1 month, but there are %d of them", len(snapmap))
	}

	for _, dt := range []data.DurationTime{upperTimeLimit, shiftUpper, dtUpper} {
		optsSnap = SnapshotOptions{SnapshotFilter: data.SnapshotFilter{
			UpperTimeLimit: dt,
			// relative-to latest
		}}
		_, snapmap := testRunSnapshotsOpts(t, env.gopts, optsSnap)
		// 1m, 2m, 6m
		rtest.Assert(t, len(snapmap) == 3, "there should be 3 snapshots older than 1 month, but there are %d of them", len(snapmap))
	}

	// ================== LowerTimeLimit ==================
	for _, dt := range []data.DurationTime{lowerTimeLimit, shiftLower, dtLower} {
		optsSnap = SnapshotOptions{SnapshotFilter: data.SnapshotFilter{
			LowerTimeLimit: dt,
			RelativeTo:     referenceTime,
		}}
		_, snapmap := testRunSnapshotsOpts(t, env.gopts, optsSnap)

		// 2m, 1m, 2d, 0h
		rtest.Assert(t, len(snapmap) == 4, "there should be 4 snapshots newer than 1 month, but there are %d of them", len(snapmap))
	}

	for _, dt := range []data.DurationTime{shiftLower, dtLower, lowerTimeLimit} {
		optsSnap = SnapshotOptions{SnapshotFilter: data.SnapshotFilter{
			LowerTimeLimit: dt,
			// relative-to latest
		}}
		_, snapmap := testRunSnapshotsOpts(t, env.gopts, optsSnap)

		// 2m, 1m, 2d, 0h
		rtest.Assert(t, len(snapmap) == 4, "there should be 4 snapshots newer than 1 month, but there are %d of them", len(snapmap))
	}

	// ================== both time filters ==================
	type both struct {
		LowerTimeLimit data.DurationTime
		UpperTimeLimit data.DurationTime
	}

	for _, dt := range []both{{lowerTimeLimit, upperTimeLimit}, {shiftLower, shiftUpper}, {dtLower, dtUpper}} {
		optsSnap = SnapshotOptions{SnapshotFilter: data.SnapshotFilter{
			LowerTimeLimit: dt.LowerTimeLimit,
			UpperTimeLimit: dt.UpperTimeLimit,
			RelativeTo:     referenceTime,
		}}
		_, snapmap = testRunSnapshotsOpts(t, env.gopts, optsSnap)
		// 1m 2m
		rtest.Assert(t, len(snapmap) == 2, "there should be 2 snapshots between 1 and 2 months, but there are %d of them", len(snapmap))
	}

	for _, dt := range []both{{lowerTimeLimit, upperTimeLimit}, {shiftLower, shiftUpper}, {dtLower, dtUpper}} {
		optsSnap = SnapshotOptions{SnapshotFilter: data.SnapshotFilter{
			LowerTimeLimit: dt.LowerTimeLimit,
			UpperTimeLimit: dt.UpperTimeLimit,
			// relative-to latest
		}}
		_, snapmap = testRunSnapshotsOpts(t, env.gopts, optsSnap)
		// 1m 2m
		rtest.Assert(t, len(snapmap) == 2, "there should be 2 snapshots between 1 and 2 months, but there are %d of them", len(snapmap))
	}

	// ================== corner cases ==================
	// case 1: LowerTimeLimit == UpperTimeLimit
	optsSnap = SnapshotOptions{SnapshotFilter: data.SnapshotFilter{
		LowerTimeLimit: shiftUpper,
		UpperTimeLimit: shiftUpper,
	}}
	_, snapmap = testRunSnapshotsOpts(t, env.gopts, optsSnap)
	rtest.Equals(t, 1, len(snapmap))
	rtest.Equals(t, durationMap[2], slices.Collect(maps.Keys(snapmap))[0])

	// case 2: take a second away from each end, there should be none left
	ttOld := shiftLower.GetTime()
	ttNew := shiftUpper.GetTime()

	// convert modified time back to data.DurationTime
	var olderTimeP1, newerTimeM1 data.DurationTime
	olderTimeP1.Set(ttOld.Add(time.Second).Format(time.DateTime))
	newerTimeM1.Set(ttNew.Add(-1 * time.Second).Format(time.DateTime))

	optsSnap = SnapshotOptions{SnapshotFilter: data.SnapshotFilter{
		LowerTimeLimit: olderTimeP1,
		UpperTimeLimit: newerTimeM1,
	}}
	_, snapmap = testRunSnapshotsOpts(t, env.gopts, optsSnap)
	rtest.Equals(t, 0, len(snapmap))
}
