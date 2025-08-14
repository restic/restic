package restic

import (
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"

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
		if !unicode.IsNumber(s) {
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

// PastTime returns past point in time where this duration is relative to time t.  If t is not given, use time.Now().
func (d Duration) PastTime(t ...time.Time) time.Time {
	var reftime time.Time
	if t == nil {
		reftime = time.Now()
	} else {
		reftime = t[0]
	}

	return reftime.AddDate(-d.Years, -d.Months, -d.Days).Add(-time.Duration(d.Hours) * time.Hour)
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
