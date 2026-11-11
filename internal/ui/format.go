package ui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/bits"
	"regexp"
	"strconv"
	"time"
	"unicode"

	"golang.org/x/text/width"

	"github.com/restic/restic/internal/errors"
)

func FormatBytes(c uint64) string {
	b := float64(c)
	switch {
	case c >= 1<<40:
		return fmt.Sprintf("%.3f TiB", b/(1<<40))
	case c >= 1<<30:
		return fmt.Sprintf("%.3f GiB", b/(1<<30))
	case c >= 1<<20:
		return fmt.Sprintf("%.3f MiB", b/(1<<20))
	case c >= 1<<10:
		return fmt.Sprintf("%.3f KiB", b/(1<<10))
	default:
		return fmt.Sprintf("%d B", c)
	}
}

// FormatPercent formats numerator/denominator as a percentage.
func FormatPercent(numerator uint64, denominator uint64) string {
	if denominator == 0 {
		return ""
	}

	percent := 100.0 * float64(numerator) / float64(denominator)
	if percent > 100 {
		percent = 100
	}

	return fmt.Sprintf("%3.2f%%", percent)
}

// FormatDuration formats d as FormatSeconds would.
func FormatDuration(d time.Duration) string {
	sec := uint64(d / time.Second)
	return FormatSeconds(sec)
}

// FormatSeconds formats sec as MM:SS, or HH:MM:SS if sec seconds
// is at least an hour.
func FormatSeconds(sec uint64) string {
	hours := sec / 3600
	sec -= hours * 3600
	mins := sec / 60
	sec -= mins * 60
	if hours > 0 {
		return fmt.Sprintf("%d:%02d:%02d", hours, mins, sec)
	}
	return fmt.Sprintf("%d:%02d", mins, sec)
}

// ParseBytes parses a size in bytes from s. It understands the suffixes
// B, K, M, G and T for powers of 1024.
func ParseBytes(s string) (int64, error) {
	if s == "" {
		return 0, errors.New("expected size, got empty string")
	}

	numStr := s[:len(s)-1]
	var unit uint64 = 1

	switch s[len(s)-1] {
	case 'b', 'B':
		// use initialized values, do nothing here
	case 'k', 'K':
		unit = 1024
	case 'm', 'M':
		unit = 1024 * 1024
	case 'g', 'G':
		unit = 1024 * 1024 * 1024
	case 't', 'T':
		unit = 1024 * 1024 * 1024 * 1024
	default:
		numStr = s
	}
	value, err := strconv.ParseInt(numStr, 10, 64)
	if err != nil {
		return 0, err
	}

	hi, lo := bits.Mul64(uint64(value), unit)
	value = int64(lo)
	if hi != 0 || value < 0 {
		return 0, fmt.Errorf("ParseSize: %q: %w", numStr, strconv.ErrRange)
	}

	return value, nil
}

func ToJSONString(status any) string {
	buf := new(bytes.Buffer)
	err := json.NewEncoder(buf).Encode(status)
	if err != nil {
		panic(err)
	}
	return buf.String()
}

// DisplayWidth returns the number of terminal cells needed to display s
func DisplayWidth(s string) int {
	width := 0
	for _, r := range s {
		width += displayRuneWidth(r)
	}

	return width
}

func displayRuneWidth(r rune) int {
	switch width.LookupRune(r).Kind() {
	case width.EastAsianWide, width.EastAsianFullwidth:
		return 2
	case width.EastAsianNarrow, width.EastAsianHalfwidth, width.EastAsianAmbiguous, width.Neutral:
		return 1
	default:
		return 0
	}
}

// Quote lines with funny characters in them, meaning control chars, newlines,
// tabs, anything else non-printable and invalid UTF-8.
//
// This is intended to produce a string that does not mess up the terminal
// rather than produce an unambiguous quoted string.
func Quote(line string) string {
	for _, r := range line {
		// The replacement character usually means the input is not UTF-8.
		if r == unicode.ReplacementChar || !unicode.IsPrint(r) {
			return strconv.Quote(line)
		}
	}
	return line
}

// Truncate s to fit in width (number of terminal cells) w.
// If w is negative, returns the empty string.
func Truncate(s string, w int) string {
	if len(s) < w {
		// Since the display width of a character is at most 2
		// and all of ASCII (single byte per rune) has width 1,
		// no character takes more bytes to encode than its width.
		return s
	}

	for i := uint(0); i < uint(len(s)); {
		utfsize := uint(1) // UTF-8 encoding size of first rune in s.
		w--

		if s[i] > unicode.MaxASCII {
			var wide bool
			if wide, utfsize = wideRune(s[i:]); wide {
				w--
			}
		}

		if w < 0 {
			return s[:i]
		}
		i += utfsize
	}

	return s
}

// Guess whether the first rune in s would occupy two terminal cells
// instead of one. This cannot be determined exactly without knowing
// the terminal font, so we treat all ambiguous runes as full-width,
// i.e., two cells.
func wideRune(s string) (wide bool, utfsize uint) {
	prop, size := width.LookupString(s)
	kind := prop.Kind()
	wide = kind != width.Neutral && kind != width.EastAsianNarrow
	return wide, uint(size)
}

// allowed formats for ParseTime
var timeFormats = []string{
	// yyyy-mm-dd variations
	"2006-1-2 15:4:5",
	"2006-1-2 15:4:5 -0700",
	"2006-1-2 15:4:5 MST",

	// dd.mm.yyyy variations
	"2-1-2006 15:4:5",
	"2-1-2006 15:4:5 -0700",
	"2-1-2006 15:4:5 MST",

	// default *nix date/time
	"Mon Jan 2 15:4:5 -0700 MST 2006",
	"Mon 2 Jan 15:4:5 -0700 MST 2006",
	"2 Jan 15:4:5 -0700 MST 2006",
	"Jan 2 15:4:5 -0700 MST 2006",
}

// ParseTime tries to transform given time string "str" into a time.Time,
// according to a variety of allowed formats
// this regular expression settles the substring timezone offset
var tzRe = regexp.MustCompile(`\s*([+\-])([0-5][0-9]):?([0-5][0-9])`)
var dtRe = regexp.MustCompile(`^(\d+)[/.\-](\d+)[/.z-](\d+)(?: (\d{1,2}):(\d{1,2})(?::(\d{1,2}))?)?`)

// func normalizeTZ converts a few possible timezone specifications into a
// standard one: " -0700"
func normalizeTZ(s string) string {
	return tzRe.ReplaceAllString(s, " ${1}${2}${3}")
}

func ParseTime(str string) (time.Time, error) {
	str = normalizeTZ(str)

	// dtRe matches a date with a possible time modifier
	// at the end a standard string is created with all 6 date / time
	// components set. Missing elements are set to "0".
	dateTimeIndex := dtRe.FindStringSubmatchIndex(str)
	if len(dateTimeIndex) > 0 {
		r := make([]string, 7)
		for i := 1; i < 7; i++ {
			if dateTimeIndex[2*i] >= 0 {
				r[i] = str[dateTimeIndex[2*i]:dateTimeIndex[2*i+1]]
			} else {
				r[i] = "0"
			}
		}

		replacementDateTime := fmt.Sprintf("%s-%s-%s %s:%s:%s", r[1], r[2], r[3], r[4], r[5], r[6])
		if dateTimeIndex[1] < len(str) {
			str = fmt.Sprintf("%s%s", replacementDateTime, str[dateTimeIndex[1]:])
		} else {
			str = replacementDateTime
		}
	}

	for _, tf := range timeFormats {
		if t, err := time.ParseInLocation(tf, str, time.Local); err == nil {
			return t, nil
		}
	}

	return time.Time{}, errors.Fatalf("unable to parse time: %q", str)
}
