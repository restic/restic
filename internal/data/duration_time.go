package data

import (
	"fmt"
	"regexp"
	"strconv"
	"time"

	"github.com/restic/restic/internal/errors"
)

// DurationTimeState describes the possible states of DurationTime struct
type DurationTimeState int

const (
	durationUninitialized DurationTimeState = iota
	durationType
	durationTimeSet
	durationSnapID
)

// DurationTime can be a Duration, a time.Time converrted from string,
// or the string `now` for a time or `latest` or an actual snapID
type DurationTime struct {
	snapID        string
	duration      Duration
	timeReference time.Time
	state         DurationTimeState
}

// Set is the interface which converts its options to one of
// a time.Time, a restic.Duration or a snapID
func (d *DurationTime) Set(s string) error {
	rDuration := regexp.MustCompile(`^(-?\d+[ymdh])+$`)
	// one or two digit month/day, time optional
	rDateTime := regexp.MustCompile(`^(\d{4})-(\d{1,2})-(\d{1,2})(?: (\d{1,2}):(\d{1,2}):(\d{1,2}))?$`)
	rSnapID := regexp.MustCompile(`^([0-9a-fA-F]{8,64}|latest)$`)
	if s == "now" {
		d.timeReference = time.Now()
		d.state = durationTimeSet
	} else if rDuration.FindString(s) == s {
		var err error
		d.duration, err = ParseDuration(s)
		if err != nil {
			return err
		}
		d.state = durationType

	} else if rDateTime.FindString(s) == s {
		match := rDateTime.FindAllStringSubmatch(s, 1)
		year, _ := strconv.Atoi(match[0][1])
		month, _ := strconv.Atoi(match[0][2])
		day, _ := strconv.Atoi(match[0][3])
		hour, _ := strconv.Atoi(match[0][4])
		minute, _ := strconv.Atoi(match[0][5])
		second, _ := strconv.Atoi(match[0][6])

		d.timeReference = time.Date(year, time.Month(month), day, hour, minute, second, 0, time.Local)
		d.state = durationTimeSet
	} else if rSnapID.FindString(s) == s {
		if len(s) > 8 {
			s = s[:8]
		}
		d.snapID = s
		d.state = durationSnapID
	} else {
		return errors.Errorf("invalid DurationTime pattern %q specified", s)
	}

	return nil
}

// Empty detects is a given DurationTime variable is not in use at all
func (d *DurationTime) Empty() bool {
	return d.state == durationUninitialized
}

// String converts the struct DurationTime to its current value
// 'pflag.Value' needs this method
func (d DurationTime) String() string {
	switch d.state {
	case durationUninitialized:
		return ""
	case durationType:
		return fmt.Sprintf("Duration(%s)", d.duration)
	case durationTimeSet:
		return fmt.Sprintf("Time(%s)", d.GetTime())
	case durationSnapID:
		return fmt.Sprintf("Snap(%s)", d.snapID)
	default:
		return "DurationTime(invalid)"
	}
}

// Type of 'DurationTime'
func (d DurationTime) Type() string {
	return "DurationTime"
}

// AddOffset add a Duration value to to a given time reference
func (d *DurationTime) AddOffset(o DurationTime) DurationTime {
	if d.state == durationTimeSet && o.state == durationType {
		var new DurationTime
		new.timeReference = d.timeReference.AddDate(-o.duration.Years, -o.duration.Months, -o.duration.Days).
			Add(time.Hour * time.Duration(-o.duration.Hours))
		new.state = durationTimeSet
		return new
	}
	return *d
}

// GetTime accesses time component of a DurationTime
func (d *DurationTime) GetTime() time.Time {
	if d.state == durationTimeSet {
		return d.timeReference
	}
	panic(fmt.Sprintf("DurationTime: the time has not been set, state=%q", d.String()))
}
