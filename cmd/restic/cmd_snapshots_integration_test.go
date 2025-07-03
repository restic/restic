package main

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
)

func testRunSnapshots(t testing.TB, gopts GlobalOptions) (newest *Snapshot, snapmap map[restic.ID]Snapshot) {
	buf, err := withCaptureStdout(func() error {
		gopts.JSON = true

		opts := SnapshotOptions{}
		return runSnapshots(context.TODO(), opts, gopts, []string{})
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

func testRunSnapshotsOpts(t testing.TB, opts SnapshotOptions, gopts GlobalOptions) (snapmap map[restic.ID]Snapshot) {
	buf, err := withCaptureStdout(func() error {
		gopts.JSON = true

		// list all snapshots in JSON mode
		return runSnapshots(context.TODO(), opts, gopts, []string{})
	})
	rtest.OK(t, err)

	snapshots := []Snapshot{}
	rtest.OK(t, json.Unmarshal(buf.Bytes(), &snapshots))

	snapmap = make(map[restic.ID]Snapshot, len(snapshots))
	for _, sn := range snapshots {
		snapmap[*sn.ID] = sn
	}
	return snapmap
}

func testSetDuration(t *testing.T, duration string) (result restic.DurationTime) {
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
	offsets := []restic.DurationTime{
		testSetDuration(t, "0h"),
		testSetDuration(t, "2d"),
		testSetDuration(t, "1m"),
		testSetDuration(t, "2m"),
		testSetDuration(t, "6m"),
	}

	// convert Durations to timestamps
	newTimes := make([]restic.DurationTime, len(offsets))
	for i, offset := range offsets {
		newTimes[i] = timeReference.AddOffset(offset)
	}

	// rewrite the backup timestamps for all given backups
	for i, snapID := range snapshotIDs {
		rewriteOpts := RewriteOptions{
			Metadata: snapshotMetadataArgs{
				Time: newTimes[i].GetTime().Format(time.DateTime),
			},
			Forget: true,
		}
		rtest.OK(t, runRewrite(context.TODO(), rewriteOpts, env.gopts, []string{snapID.String()}))
	}
	testListSnapshots(t, env.gopts, 5)

	// extract all snapshots and their backup timestamps into
	referenceT := testSetDuration(t, "now")
	latest := testSetDuration(t, "latest")

	optsSnap := SnapshotOptions{
		SnapshotFilter: restic.SnapshotFilter{
			OlderThan:  latest,
			RelativeTo: referenceT,
		},
	}
	snapmap := testRunSnapshotsOpts(t, optsSnap, env.gopts)
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

	var dtOlder restic.DurationTime
	var dtNewer restic.DurationTime
	rtest.OK(t, dtOlder.Set(durationMap[2].String()))
	rtest.OK(t, dtNewer.Set(durationMap[3].String()))

	// ================== OlderThan ==================
	// the 3 variables here represent the same timestamp in the possible 3 formats
	// a Duration, a timestamp (as calculated from an Duration offset), a snapID from a rewritten backup
	for _, dt := range []restic.DurationTime{olderThan, shiftOlder, dtOlder} {
		optsSnap = SnapshotOptions{SnapshotFilter: restic.SnapshotFilter{
			OlderThan: dt,
		}}
		snapmap = testRunSnapshotsOpts(t, optsSnap, env.gopts)
		rtest.Assert(t, len(snapmap) == 3, "there should be 3 snapshots older than 1 month, but there are %d of them", len(snapmap))
	}

	// ================== NewerThan ==================
	for _, dt := range []restic.DurationTime{newerThan, shiftNewer, dtNewer} {
		optsSnap = SnapshotOptions{SnapshotFilter: restic.SnapshotFilter{
			NewerThan: dt,
		}}
		snapmap = testRunSnapshotsOpts(t, optsSnap, env.gopts)
		rtest.Assert(t, len(snapmap) == 4, "there should be 4 snapshots older than 1 month, but there are %d of them", len(snapmap))
	}

	// ================== both time filters ==================
	type both struct {
		NewerThan restic.DurationTime
		OlderThan restic.DurationTime
	}

	for _, elem := range []both{{newerThan, olderThan}, {shiftNewer, shiftOlder}, {dtNewer, dtOlder}} {
		optsSnap = SnapshotOptions{SnapshotFilter: restic.SnapshotFilter{
			NewerThan: elem.NewerThan,
			OlderThan: elem.OlderThan,
		}}
		snapmap = testRunSnapshotsOpts(t, optsSnap, env.gopts)
		rtest.Assert(t, len(snapmap) == 2, "there should be 2 snapshots between 1 and 2 months, but there are %d of them", len(snapmap))
	}
}
