package data

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/restic/restic/internal/errors"
)

// Duration is similar to time.Duration, except it only supports larger ranges
// like hours, days, months, and years.
type Duration struct {
	Hours, Days, Months, Years int
}

func (d Duration) String() string {
	var s string
	if d.Years != 0 {
		s += fmt.Sprintf("%dy", d.Years)
	}

	if d.Months != 0 {
		s += fmt.Sprintf("%dm", d.Months)
	}

	if d.Days != 0 {
		s += fmt.Sprintf("%dd", d.Days)
	}

	if d.Hours != 0 {
		s += fmt.Sprintf("%dh", d.Hours)
	}

	return s
}

func nextNumber(input string) (num int, rest string, err error) {
	if len(input) == 0 {
		return 0, "", nil
	}

	var (
		n        string
		negative bool
	)

	if input[0] == '-' {
		negative = true
		input = input[1:]
	}

	for i, s := range input {
		if s < '0' || s > '9' {
			rest = input[i:]
			break
		}

		n += string(s)
	}

	if len(n) == 0 {
		return 0, input, errors.New("no number found")
	}

	num, err = strconv.Atoi(n)
	if err != nil {
		panic(err)
	}

	if negative {
		num = -num
	}

	return num, rest, nil
}

// ParseDuration parses a duration from a string. The format is `6y5m234d37h`
func ParseDuration(s string) (Duration, error) {
	var (
		d   Duration
		num int
		err error
	)

	s = strings.TrimSpace(s)

	for s != "" {
		num, s, err = nextNumber(s)
		if err != nil {
			return Duration{}, err
		}

		if len(s) == 0 {
			return Duration{}, errors.Errorf("no unit found after number %d", num)
		}

		switch s[0] {
		case 'y':
			d.Years = num
		case 'm':
			d.Months = num
		case 'd':
			d.Days = num
		case 'h':
			d.Hours = num
		default:
			return Duration{}, errors.Errorf("invalid unit %q found after number %d", s[0], num)
		}

		s = s[1:]
	}

	return d, nil
}

// Set calls ParseDuration and updates d.
func (d *Duration) Set(s string) error {
	v, err := ParseDuration(s)
	if err != nil {
		return err
	}

	*d = v
	return nil
}

// Type returns the type of Duration, usable within github.com/spf13/pflag and
// in help texts.
func (d Duration) Type() string {
	return "duration"
}

// Zero returns true if the duration is empty (all values are set to zero).
func (d Duration) Zero() bool {
	return d.Years == 0 && d.Months == 0 && d.Days == 0 && d.Hours == 0
}

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
	name          string
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

func (d DurationTime) GetName() string {
	return d.name
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
