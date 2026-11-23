package data

import (
	"fmt"
	"regexp"
	"time"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/ui"
)

// DurationTimeState describes the possible states of DurationTime struct
type DurationTimeState int

const (
	durationUninitialized DurationTimeState = iota
	durationType
	durationTimeSet
	durationSnapID
)

// DurationTime can be a Duration, a time.Time converted from string,
// or the string `now` for a time or `latest` or an actual snapID
type DurationTime struct {
	snapID        string
	duration      Duration
	timeReference time.Time
	state         DurationTimeState
}

// a data.Duration is
// one or more instances of "optional minus sign, followed by a number, followed by one of "[ymdh]"
var rDuration = regexp.MustCompile(`^(-?\d+[ymdh])+$`)

// snapshot ID, between 8 and 64 characters long. or the string "latest"
var rSnapID = regexp.MustCompile(`^([0-9a-fA-F]{8,64}|latest)$`)

// Set is the interface which converts its options to one of
// a time.Time, a restic.Duration or a snapshot ID
func (d *DurationTime) Set(s string) error {
	switch {
	case s == "now":
		d.timeReference = time.Now()
		d.state = durationTimeSet

	case rSnapID.FindString(s) == s:
		if len(s) > 8 {
			s = s[:8]
		}
		d.snapID = s
		d.state = durationSnapID

	case rDuration.FindString(s) == s:
		dd, err := ParseDuration(s)
		if err != nil {
			return err
		}
		d.duration = dd
		d.state = durationType

	default:
		tt, err := ui.ParseTime(s)
		if err != nil {
			return errors.Errorf("invalid DurationTime pattern %q specified", s)
		}
		d.timeReference = tt
		d.state = durationTimeSet
	}

	return nil
}

// Empty detects if a given DurationTime variable is not used at all
func (d *DurationTime) Empty() bool {
	return d.state == durationUninitialized
}

// String converts 'DurationTime' to its current value
func (d DurationTime) String() string {
	switch d.state {
	case durationUninitialized:
		return ""
	case durationType:
		return fmt.Sprintf("Duration(%s)", d.duration)
	case durationTimeSet:
		return d.GetTime().String()
	case durationSnapID:
		return fmt.Sprintf("Snap(%s)", d.snapID)
	default:
		return "DurationTime(invalid)"
	}
}

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
	} else if d.state != durationTimeSet {
		panic(fmt.Sprintf("invalid state for time reference: %v", d))
	} else if o.state != durationType {
		panic(fmt.Sprintf("invalid state for duration offset: %v", o))
	}
	return DurationTime{}
}

// GetTime accesses time component of a DurationTime
func (d *DurationTime) GetTime() time.Time {
	if d.state == durationTimeSet {
		return d.timeReference.Local()
	}
	panic(fmt.Sprintf("DurationTime: the time has not been set, state=%q", d))
}
