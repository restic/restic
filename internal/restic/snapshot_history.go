package restic

import (
	"fmt"
	"github.com/restic/restic/internal/ui/table"
	"io"
	"sort"
	"time"
)

// SnapshotHistory is the history of the snapshots and has to be similar to the expiration policy.
type SnapshotHistory struct {
	CurrentHour              bool // was there a snapshot for the current hour found
	ConsecutiveHourlyBackups int  // how many consecutive hourly snapshots are present? - not counting the current hour

	CurrentDay              bool
	ConsecutiveDailyBackups int // how many daily snapshots are present?

	CurrentWeek              bool
	ConsecutiveWeeklyBackups int // how many weekly snapshots are present?

	CurrentMonth              bool
	ConsecutiveMonthlyBackups int // how many monthly snapshots are present?

	CurrentYear              bool
	ConsecutiveYearlyBackups int // how many yearly snapshots are present?
}

// copied from snapshot policy
// ymdh returns an integer in the form YYYYMMDDHH.
func hourStampInteger(d time.Time) int {
	return d.Year()*1000000 + int(d.Month())*10000 + d.Day()*100 + d.Hour()
}

// ymdh returns an integer in the form YYYYMMDDHH.
func previousHour(t time.Time) time.Time {
	return t.Add(time.Duration(-1) * time.Hour)
}

// ymd returns an integer in the form YYYYMMDD.
func dayStampInteger(d time.Time) int {
	return d.Year()*10000 + int(d.Month())*100 + d.Day()
}

func previousDay(t time.Time) time.Time {
	return t.AddDate(0, 0, -1)
}

// yw returns an integer in the form YYYYWW, where WW is the week number.
func weekStampInteger(d time.Time) int {
	year, week := d.ISOWeek()
	return year*100 + week
}

func previousWeek(t time.Time) time.Time {
	return t.AddDate(0, 0, -7)
}

// ym returns an integer in the form YYYYMM.
func monthStampInteger(d time.Time) int {
	return d.Year()*100 + int(d.Month())
}

func previousMonth(t time.Time) time.Time {
	return t.AddDate(0, -1, 0)
}

// y returns the year of d.
func yearStampInteger(d time.Time) int {
	return d.Year()
}

func previousYear(t time.Time) time.Time {
	return t.AddDate(-1, 0, 0)
}

type scanningMode struct {
	timeToPattern      func(time.Time) int
	previousTime       func(time.Time) time.Time
	currentFound       func(*SnapshotHistory)
	incremementOnFound func(*SnapshotHistory)
}

type scanningModes []scanningMode

func foundCurrentHour(s *SnapshotHistory) {
	s.CurrentHour = true
}

func foundHour(s *SnapshotHistory) {
	s.ConsecutiveHourlyBackups++
}

func foundCurrentDay(s *SnapshotHistory) {
	s.CurrentDay = true
}

func foundDay(s *SnapshotHistory) {
	s.ConsecutiveDailyBackups++
}

func foundCurrentWeek(s *SnapshotHistory) {
	s.CurrentWeek = true
}

func foundWeek(s *SnapshotHistory) {
	s.ConsecutiveWeeklyBackups++
}

func foundCurrentMonth(s *SnapshotHistory) {
	s.CurrentMonth = true
}

func foundMonth(s *SnapshotHistory) {
	s.ConsecutiveMonthlyBackups++
}

func foundCurrentYear(s *SnapshotHistory) {
	s.CurrentYear = true
}

func foundYear(s *SnapshotHistory) {
	s.ConsecutiveYearlyBackups++
}

// BuildHistory extracts the SnapshotHistory from a list of Snapshots
func BuildHistory(currentTime time.Time, snapshots Snapshots) SnapshotHistory {
	// the snapshots need to be ordered
	sort.Sort(snapshots)

	var scanningModes scanningModes

	scanningModes = append(scanningModes, scanningMode{hourStampInteger, previousHour, foundCurrentHour, foundHour})
	scanningModes = append(scanningModes, scanningMode{dayStampInteger, previousDay, foundCurrentDay, foundDay})
	scanningModes = append(scanningModes, scanningMode{weekStampInteger, previousWeek, foundCurrentWeek, foundWeek})
	scanningModes = append(scanningModes, scanningMode{monthStampInteger, previousMonth, foundCurrentMonth, foundMonth})
	scanningModes = append(scanningModes, scanningMode{yearStampInteger, previousYear, foundCurrentYear, foundYear})

	result := SnapshotHistory{}

	for _, scanningMode := range scanningModes {

		comparisonTime := currentTime
		current := true
		var continueScan bool

		for _, snapshot := range snapshots {

			for {
				continueScan, comparisonTime = check(snapshot, comparisonTime, scanningMode, current, &result)
				current = false
				if !continueScan {
					break
				}
			}
		}
	}
	return result
}

func check(snapshot *Snapshot, comparisonTime time.Time, scanningMode scanningMode, current bool, snapshotHistory *SnapshotHistory) (bool, time.Time) {

	stampToCompare := scanningMode.timeToPattern(comparisonTime)
	snapshotStamp := scanningMode.timeToPattern(snapshot.Time)

	if stampToCompare == snapshotStamp {
		if current {
			scanningMode.currentFound(snapshotHistory)
		} else {
			scanningMode.incremementOnFound(snapshotHistory)
		}
		return true, scanningMode.previousTime(comparisonTime)
	} else if current {
		// test previous time
		return true, scanningMode.previousTime(comparisonTime)
	}

	return false, comparisonTime
}

func PrintHistory(stdout io.Writer, history SnapshotHistory) {
	fmt.Fprint(stdout, "\nSnapshot History\n")
	tab := table.New()

	tab.AddColumn("Period", "{{ .Period }}")
	tab.AddColumn("Current", "{{ .Current }}")
	tab.AddColumn("Consecutive Previous Backups", "{{ .Consecutive }}")

	previousCurrentPrinted := false
	previousCurrentPrinted = appendRow(tab, "Hourly", previousCurrentPrinted, history.CurrentHour, history.ConsecutiveHourlyBackups)
	previousCurrentPrinted = appendRow(tab, "Daily", previousCurrentPrinted, history.CurrentDay, history.ConsecutiveDailyBackups)
	previousCurrentPrinted = appendRow(tab, "Weekly", previousCurrentPrinted, history.CurrentWeek, history.ConsecutiveWeeklyBackups)
	previousCurrentPrinted = appendRow(tab, "Monthly", previousCurrentPrinted, history.CurrentMonth, history.ConsecutiveMonthlyBackups)
	previousCurrentPrinted = appendRow(tab, "Yearly", previousCurrentPrinted, history.CurrentYear, history.ConsecutiveYearlyBackups)

	tab.Write(stdout)
}

type row struct {
	Period      string
	Current     bool
	Consecutive int
}

func appendRow(table *table.Table, title string, previousCurrentPrinted bool, current bool, consecutiveCount int) bool {
	// skip all that have nothing to display
	if !current && consecutiveCount == 0 {
		return previousCurrentPrinted
	}
	// skip when no consecutive history and current already printed (current hour implies current day!
	if consecutiveCount <= 1 && previousCurrentPrinted {
		return previousCurrentPrinted
	}
	table.AddRow(row{
		Period:      title,
		Current:     current,
		Consecutive: consecutiveCount,
	})
	return current || previousCurrentPrinted
}
