package data_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/restic/restic/internal/data"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/test"
)

func TestFindLatestSnapshot(t *testing.T) {
	repo := repository.TestRepository(t)
	data.TestCreateSnapshot(t, repo, parseTimeUTC("2015-05-05 05:05:05"), 1)
	data.TestCreateSnapshot(t, repo, parseTimeUTC("2017-07-07 07:07:07"), 1)
	latestSnapshot := data.TestCreateSnapshot(t, repo, parseTimeUTC("2019-09-09 09:09:09"), 1)

	f := data.SnapshotFilter{Hosts: []string{"foo"}}
	sn, _, err := f.FindLatest(context.TODO(), repo, repo, "latest")
	if err != nil {
		t.Fatalf("FindLatest returned error: %v", err)
	}

	if *sn.ID() != *latestSnapshot.ID() {
		t.Errorf("FindLatest returned wrong snapshot ID: %v", *sn.ID())
	}
}

func TestFindLatestSnapshotWithMaxTimestamp(t *testing.T) {
	repo := repository.TestRepository(t)
	data.TestCreateSnapshot(t, repo, parseTimeUTC("2015-05-05 05:05:05"), 1)
	desiredSnapshot := data.TestCreateSnapshot(t, repo, parseTimeUTC("2017-07-07 07:07:07"), 1)
	data.TestCreateSnapshot(t, repo, parseTimeUTC("2019-09-09 09:09:09"), 1)

	sn, _, err := (&data.SnapshotFilter{
		Hosts:          []string{"foo"},
		TimestampLimit: parseTimeUTC("2018-08-08 08:08:08"),
	}).FindLatest(context.TODO(), repo, repo, "latest")
	if err != nil {
		t.Fatalf("FindLatest returned error: %v", err)
	}

	if *sn.ID() != *desiredSnapshot.ID() {
		t.Errorf("FindLatest returned wrong snapshot ID: %v", *sn.ID())
	}
}

func TestFindLatestWithSubpath(t *testing.T) {
	repo := repository.TestRepository(t)
	data.TestCreateSnapshot(t, repo, parseTimeUTC("2015-05-05 05:05:05"), 1)
	desiredSnapshot := data.TestCreateSnapshot(t, repo, parseTimeUTC("2017-07-07 07:07:07"), 1)

	for _, exp := range []struct {
		query     string
		subfolder string
	}{
		{"latest", ""},
		{"latest:subfolder", "subfolder"},
		{desiredSnapshot.ID().Str(), ""},
		{desiredSnapshot.ID().Str() + ":subfolder", "subfolder"},
		{desiredSnapshot.ID().String(), ""},
		{desiredSnapshot.ID().String() + ":subfolder", "subfolder"},
	} {
		t.Run("", func(t *testing.T) {
			sn, subfolder, err := (&data.SnapshotFilter{}).FindLatest(context.TODO(), repo, repo, exp.query)
			if err != nil {
				t.Fatalf("FindLatest returned error: %v", err)
			}

			test.Assert(t, *sn.ID() == *desiredSnapshot.ID(), "FindLatest returned wrong snapshot ID: %v", *sn.ID())
			test.Assert(t, subfolder == exp.subfolder, "FindLatest returned wrong path in snapshot: %v", subfolder)
		})
	}
}

func TestFindAllSubpathError(t *testing.T) {
	repo := repository.TestRepository(t)
	desiredSnapshot := data.TestCreateSnapshot(t, repo, parseTimeUTC("2017-07-07 07:07:07"), 1)

	count := 0
	test.OK(t, (&data.SnapshotFilter{}).FindAll(context.TODO(), repo, repo,
		[]string{"latest:subfolder", desiredSnapshot.ID().Str() + ":subfolder"},
		func(id string, sn *data.Snapshot, err error) error {
			if err == data.ErrInvalidSnapshotSyntax {
				count++
				return nil
			}
			return err
		}))
	test.Assert(t, count == 2, "unexpected number of subfolder errors: %v, wanted %v", count, 2)
}

func TestDurationTimePattern(t *testing.T) {
	// duration as string and equivalent number of hours
	type TimeOffsetResult struct {
		duration      string
		durationHours int
	}

	referenceTime := data.DurationTime{}
	test.OK(t, referenceTime.Set("2025-1-1"))
	timeStamp := referenceTime.GetTime()

	timeOffsets := []TimeOffsetResult{
		{"-2h", -2},
		{"1d1h", 25},
		{"1h1d", 25},
		{"4h", 4},
		{"1d-2h", 22},
		{"-1d-2h", -26},
		{"30d24h", 31 * 24},
		{"24h30d", 31 * 24},
		{"1m", 31 * 24},
		{"2m", (31 + 30) * 24},   // Nov 2024 + Dec 2024
		{"-2m", -(31 + 28) * 24}, // Jan 2025 + Feb 2025
		{"1y", 366 * 24},         // 2024 was a leap year
	}

	for i, offset := range timeOffsets {
		temp := data.DurationTime{}
		test.OK(t, temp.Set(offset.duration))
		temp = referenceTime.AddOffset(temp)

		asTime := temp.GetTime()
		diff := timeStamp.Sub(asTime).Hours()
		test.Assert(t, diff == float64(timeOffsets[i].durationHours),
			"test %d expected %f hours difference, but got %f hours difference",
			i, float64(offset.durationHours), diff)
	}
}

func TestDurationTimeDiff(t *testing.T) {
	// this tests the conversion of a DurationTime into a time.Time
	// and the function GetTime(), Set(), String(), AddOffset()
	type TimeOffsetResult struct {
		duration      string
		durationHours int
	}

	referenceTime := data.DurationTime{}
	test.OK(t, referenceTime.Set("2025-01-01"))
	timeOffsets := []TimeOffsetResult{
		{"-2h", -2},
		{"1d1h", 25},
		{"4h", 4},
		{"1d-2h", 22},
		{"-1d-2h", -26},
		{"30d24h", 31 * 24},
		{"1m", 31 * 24},
		{"2m", (31 + 30) * 24},   // Nov 2024 + Dec 2024
		{"-2m", -(31 + 28) * 24}, // Jan 2025 + Feb 2025
		{"1y", 366 * 24},         // 2024 was a leap year
	}
	timeDurations := make([]data.DurationTime, 0, len(timeOffsets))
	for i, offset := range timeOffsets {
		temp := data.DurationTime{}
		test.OK(t, temp.Set(offset.duration))
		temp2 := referenceTime.AddOffset(temp)
		timeDurations = append(timeDurations, temp2)

		// string representation
		str := temp.String()
		tt := fmt.Sprintf("Duration(%s)", offset.duration)
		test.Assert(t, str == tt,
			"test %d expected '%s', but got 'Duration(%s)'", i, tt, str)
	}

	timeStamp := referenceTime.GetTime()
	for i, elem := range timeDurations {
		asTime := elem.GetTime()
		diff := timeStamp.Sub(asTime).Hours()
		test.Assert(t, diff == float64(timeOffsets[i].durationHours),
			"test %d expected %f hours difference, but got %f hours difference",
			i, float64(timeOffsets[i].durationHours), diff)
	}
}
