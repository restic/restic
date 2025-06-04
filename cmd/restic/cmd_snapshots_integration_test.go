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

		return runSnapshots(context.TODO(), opts, gopts, []string{})
	})
	rtest.OK(t, err)

	snapshots := []Snapshot{}
	rtest.OK(t, json.Unmarshal(buf.Bytes(), &snapshots))

	snapmap = make(map[restic.ID]Snapshot, len(snapshots))
	for _, sn := range snapshots {
		snapmap[*sn.ID] = sn
	}
	return
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

	// make 5 backups
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
	newTimes := make([]restic.DurationTime, len(offsets))
	for i, offset := range offsets {
		newTimes[i] = timeReference.AddOffset(offset)
	}

	// rewrite the backup times for all given backups
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

	// extract all snapshots and their time
	optsSnap := SnapshotOptions{
		SnapshotFilter: restic.SnapshotFilter{
			OlderThan:  offsets[0],
			RelativeTo: timeReference,
		},
	}
	snapmap := testRunSnapshotsOpts(t, optsSnap, env.gopts)
	rtest.Assert(t, len(snapmap) == 5, "there should be 5 snapshots in all, but there are %d of them", len(snapmap))

	durationMap := map[int]restic.ID{}
	for ID, sn := range snapmap {
		for i, newTime := range newTimes {
			if newTime.GetTime().Equal(sn.Time) {
				durationMap[i] = ID
				break
			}
		}
	}
	rtest.Assert(t, len(durationMap) == 5, "there should be 5 entries in 'durationMap', but there are %d of them", len(durationMap))

	olderThan := offsets[2]
	newerThan := offsets[3]
	var dtOlder restic.DurationTime
	var dtNewer restic.DurationTime
	rtest.OK(t, dtOlder.Set(durationMap[2].String()))
	rtest.OK(t, dtNewer.Set(durationMap[3].String()))

	shiftOlder := timeReference.AddOffset(olderThan)
	shiftOlder.SetTime(shiftOlder.GetTime().Add(time.Second))
	shiftNewer := timeReference.AddOffset(newerThan)
	shiftNewer.SetTime(shiftNewer.GetTime().Add(-time.Second))

	// if 'RelativeTo' is missing from 'SnapshotOptions', its either because
	// the other options have a fixed restic.DurationTime or are implicitely
	// activating RelativeTo = latest, the default setting

	// ================== OlderThan ==================
	optsSnap = SnapshotOptions{
		SnapshotFilter: restic.SnapshotFilter{
			OlderThan:  olderThan,
			RelativeTo: timeReference,
		},
	}
	snapmap = testRunSnapshotsOpts(t, optsSnap, env.gopts)
	rtest.Assert(t, len(snapmap) == 3, "there should be 3 snapshots older than 1 month, but there are %d of them", len(snapmap))

	for _, dt := range []restic.DurationTime{olderThan, shiftOlder, dtOlder} {
		optsSnap = SnapshotOptions{SnapshotFilter: restic.SnapshotFilter{
			OlderThan: dt,
		}}
		snapmap = testRunSnapshotsOpts(t, optsSnap, env.gopts)
		rtest.Assert(t, len(snapmap) == 3, "there should be 3 snapshots older than 1 month, but there are %d of them", len(snapmap))
	}

	// ================== NewerThan ==================
	optsSnap = SnapshotOptions{
		SnapshotFilter: restic.SnapshotFilter{
			NewerThan:  newerThan,
			RelativeTo: timeReference,
		},
	}
	snapmap = testRunSnapshotsOpts(t, optsSnap, env.gopts)
	rtest.Assert(t, len(snapmap) == 4, "there should be 4 snapshots newer than 2 months, but there are %d of them", len(snapmap))

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
	optsSnap = SnapshotOptions{SnapshotFilter: restic.SnapshotFilter{
		NewerThan:  newerThan,
		OlderThan:  olderThan,
		RelativeTo: timeReference,
	}}
	snapmap = testRunSnapshotsOpts(t, optsSnap, env.gopts)
	rtest.Assert(t, len(snapmap) == 2, "there should be 2 snapshots between 1 and 2 months, but there are %d of them", len(snapmap))

	for _, elem := range []both{{newerThan, olderThan}, {shiftNewer, shiftOlder}, {dtNewer, dtOlder}} {
		optsSnap = SnapshotOptions{SnapshotFilter: restic.SnapshotFilter{
			NewerThan: elem.NewerThan,
			OlderThan: elem.OlderThan,
		}}
		snapmap = testRunSnapshotsOpts(t, optsSnap, env.gopts)
		rtest.Assert(t, len(snapmap) == 2, "there should be 2 snapshots between 1 and 2 months, but there are %d of them", len(snapmap))
	}
}
