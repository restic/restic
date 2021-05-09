package restic_test

import (
	"bytes"
	"reflect"
	"testing"
	"time"

	"github.com/restic/restic/internal/restic"
)

func TestBuildHistory(t *testing.T) {
	var snapshots restic.Snapshots
	snapshots = append(snapshots, &restic.Snapshot{Time: time.Date(2018, 01, 1, 9, 15, 0, 0, time.UTC)})
	snapshots = append(snapshots, &restic.Snapshot{Time: time.Date(2020, 01, 1, 9, 15, 0, 0, time.UTC)})
	snapshots = append(snapshots, &restic.Snapshot{Time: time.Date(2020, 01, 1, 8, 15, 0, 0, time.UTC)})
	snapshots = append(snapshots, &restic.Snapshot{Time: time.Date(2019, 01, 1, 9, 15, 0, 0, time.UTC)})

	fakeNow := time.Date(2020, 01, 1, 10, 20, 0, 0, time.UTC)
	history := restic.BuildHistory(fakeNow, snapshots)

	historyToBe := restic.SnapshotHistory{
		CurrentHour:               false,
		ConsecutiveHourlyBackups:  2,
		CurrentDay:                true,
		ConsecutiveDailyBackups:   0,
		CurrentWeek:               true,
		ConsecutiveWeeklyBackups:  0,
		CurrentMonth:              true,
		ConsecutiveMonthlyBackups: 0,
		CurrentYear:               true,
		ConsecutiveYearlyBackups:  2,
	}

	if !reflect.DeepEqual(historyToBe, history) {
		t.Error("history was not calculated correctly")
	}
}

func TestPrintHistoryNoCurrentHourly(t *testing.T) {
	historyToPrint := restic.SnapshotHistory{
		CurrentHour:               false,
		ConsecutiveHourlyBackups:  2,
		CurrentDay:                true,
		ConsecutiveDailyBackups:   0,
		CurrentWeek:               true,
		ConsecutiveWeeklyBackups:  0,
		CurrentMonth:              true,
		ConsecutiveMonthlyBackups: 0,
		CurrentYear:               true,
		ConsecutiveYearlyBackups:  2,
	}
	buf := new(bytes.Buffer)
	restic.PrintHistory(buf, historyToPrint)
	is := buf.String()

	tobe := `
Snapshot History
Period  Current  Consecutive Previous Backups
---------------------------------------------
Hourly  false    2
Daily   true     0
Yearly  true     2
---------------------------------------------
`
	if tobe != is {
		t.Errorf("history was not printed correctly %v\n", is)
	}
}

func TestPrintHistoryCurrentDaily01(t *testing.T) {
	historyToPrint := restic.SnapshotHistory{
		CurrentHour:               true,
		ConsecutiveHourlyBackups:  0,
		CurrentDay:                true,
		ConsecutiveDailyBackups:   7,
		CurrentWeek:               true,
		ConsecutiveWeeklyBackups:  0,
		CurrentMonth:              true,
		ConsecutiveMonthlyBackups: 0,
		CurrentYear:               true,
		ConsecutiveYearlyBackups:  0,
	}
	buf := new(bytes.Buffer)
	restic.PrintHistory(buf, historyToPrint)
	is := buf.String()

	tobe := `
Snapshot History
Period  Current  Consecutive Previous Backups
---------------------------------------------
Hourly  true     0
Daily   true     7
---------------------------------------------
`
	if tobe != is {
		t.Errorf("history was not printed correctly %v\n", is)
	}
}

func TestPrintHistoryCurrentDaily02(t *testing.T) {
	historyToPrint := restic.SnapshotHistory{
		CurrentHour:               false,
		ConsecutiveHourlyBackups:  0,
		CurrentDay:                true,
		ConsecutiveDailyBackups:   7,
		CurrentWeek:               true,
		ConsecutiveWeeklyBackups:  0,
		CurrentMonth:              true,
		ConsecutiveMonthlyBackups: 0,
		CurrentYear:               true,
		ConsecutiveYearlyBackups:  0,
	}
	buf := new(bytes.Buffer)
	restic.PrintHistory(buf, historyToPrint)
	is := buf.String()

	tobe := `
Snapshot History
Period  Current  Consecutive Previous Backups
---------------------------------------------
Daily   true     7
---------------------------------------------
`
	if tobe != is {
		t.Errorf("history was not printed correctly %v\n", is)
	}
}

func TestNewHourlyBackupRunningFindFor12HoursWithoutCurrent(t *testing.T) {
	historyToPrint := restic.SnapshotHistory{
		CurrentHour:               false,
		ConsecutiveHourlyBackups:  12,
		CurrentDay:                true,
		ConsecutiveDailyBackups:   0,
		CurrentWeek:               true,
		ConsecutiveWeeklyBackups:  0,
		CurrentMonth:              true,
		ConsecutiveMonthlyBackups: 0,
		CurrentYear:               true,
		ConsecutiveYearlyBackups:  0,
	}
	buf := new(bytes.Buffer)
	restic.PrintHistory(buf, historyToPrint)
	is := buf.String()

	tobe := `
Snapshot History
Period  Current  Consecutive Previous Backups
---------------------------------------------
Hourly  false    12
Daily   true     0
---------------------------------------------
`
	if tobe != is {
		t.Errorf("history was not printed correctly %v\n", is)
	}
}

func TestNewHourlyBackupRunningFineFor12HoursWithCurrent(t *testing.T) {
	historyToPrint := restic.SnapshotHistory{
		CurrentHour:               true,
		ConsecutiveHourlyBackups:  12,
		CurrentDay:                true,
		ConsecutiveDailyBackups:   0,
		CurrentWeek:               true,
		ConsecutiveWeeklyBackups:  0,
		CurrentMonth:              true,
		ConsecutiveMonthlyBackups: 0,
		CurrentYear:               true,
		ConsecutiveYearlyBackups:  0,
	}
	buf := new(bytes.Buffer)
	restic.PrintHistory(buf, historyToPrint)
	is := buf.String()

	tobe := `
Snapshot History
Period  Current  Consecutive Previous Backups
---------------------------------------------
Hourly  true     12
---------------------------------------------
`
	if tobe != is {
		t.Errorf("history was not printed correctly %v\n", is)
	}
}

func TestNewHourlyBackupRunningFineFor12Hours3Days(t *testing.T) {
	historyToPrint := restic.SnapshotHistory{
		CurrentHour:               true,
		ConsecutiveHourlyBackups:  24,
		CurrentDay:                true,
		ConsecutiveDailyBackups:   3,
		CurrentWeek:               true,
		ConsecutiveWeeklyBackups:  0,
		CurrentMonth:              true,
		ConsecutiveMonthlyBackups: 0,
		CurrentYear:               true,
		ConsecutiveYearlyBackups:  0,
	}
	buf := new(bytes.Buffer)
	restic.PrintHistory(buf, historyToPrint)
	is := buf.String()

	tobe := `
Snapshot History
Period  Current  Consecutive Previous Backups
---------------------------------------------
Hourly  true     24
Daily   true     3
---------------------------------------------
`
	if tobe != is {
		t.Errorf("history was not printed correctly %v\n", is)
	}
}

func TestNewHourlyBackupRunningBrokenAfter4Days01(t *testing.T) {
	// broken for more than one hour...
	historyToPrint := restic.SnapshotHistory{
		CurrentHour:               false,
		ConsecutiveHourlyBackups:  0,
		CurrentDay:                true,
		ConsecutiveDailyBackups:   3,
		CurrentWeek:               true,
		ConsecutiveWeeklyBackups:  0,
		CurrentMonth:              true,
		ConsecutiveMonthlyBackups: 0,
		CurrentYear:               true,
		ConsecutiveYearlyBackups:  0,
	}
	buf := new(bytes.Buffer)
	restic.PrintHistory(buf, historyToPrint)
	is := buf.String()

	tobe := `
Snapshot History
Period  Current  Consecutive Previous Backups
---------------------------------------------
Daily   true     3
---------------------------------------------
`
	if tobe != is {
		t.Errorf("history was not printed correctly %v\n", is)
	}
}

func TestNewHourlyBackupRunningBrokenAfter4Days02(t *testing.T) {
	// broken for more than one day...
	historyToPrint := restic.SnapshotHistory{
		CurrentHour:               false,
		ConsecutiveHourlyBackups:  0,
		CurrentDay:                false,
		ConsecutiveDailyBackups:   0,
		CurrentWeek:               true,
		ConsecutiveWeeklyBackups:  0,
		CurrentMonth:              true,
		ConsecutiveMonthlyBackups: 0,
		CurrentYear:               true,
		ConsecutiveYearlyBackups:  0,
	}
	buf := new(bytes.Buffer)
	restic.PrintHistory(buf, historyToPrint)
	is := buf.String()

	tobe := `
Snapshot History
Period  Current  Consecutive Previous Backups
---------------------------------------------
Weekly  true     0
---------------------------------------------
`
	if tobe != is {
		t.Errorf("history was not printed correctly %v\n", is)
	}
}
