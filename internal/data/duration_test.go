package data

import (
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/restic/restic/internal/test"
)

func TestNextNumber(t *testing.T) {
	var tests = []struct {
		input string
		num   int
		rest  string
		err   bool
	}{
		{
			input: "12h", num: 12, rest: "h",
		},
		{
			input: "3d", num: 3, rest: "d",
		},
		{
			input: "4d9h", num: 4, rest: "d9h",
		},
		{
			input: "7m5d", num: 7, rest: "m5d",
		},
		{
			input: "-23y7m5d", num: -23, rest: "y7m5d",
		},
		{
			input: "-13y5m11d12h", num: -13, rest: "y5m11d12h",
		},
		{
			input: "  5d", num: 0, rest: "  5d", err: true,
		},
		{
			input: "5d  ", num: 5, rest: "d  ",
		},
		{
			input: "5", num: 5, rest: "",
		},
	}

	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			num, rest, err := nextNumber(test.input)

			if err != nil && !test.err {
				t.Fatal(err)
			}

			if num != test.num {
				t.Errorf("wrong num, want %d, got %d", test.num, num)
			}

			if rest != test.rest {
				t.Errorf("wrong rest, want %q, got %q", test.rest, rest)
			}
		})
	}
}

func TestParseDuration(t *testing.T) {
	var tests = []struct {
		input  string
		d      Duration
		output string
		err    bool
	}{
		{input: "9h", d: Duration{Hours: 9}, output: "9h"},
		{input: "3d", d: Duration{Days: 3}, output: "3d"},
		{input: "4d2h", d: Duration{Days: 4, Hours: 2}, output: "4d2h"},
		{input: "7m5d", d: Duration{Months: 7, Days: 5}, output: "7m5d"},
		{input: "6m4d8h", d: Duration{Months: 6, Days: 4, Hours: 8}, output: "6m4d8h"},
		{input: "5d7m", d: Duration{Months: 7, Days: 5}, output: "7m5d"},
		{input: "4h3d9m", d: Duration{Months: 9, Days: 3, Hours: 4}, output: "9m3d4h"},
		{input: "-7m5d", d: Duration{Months: -7, Days: 5}, output: "-7m5d"},
		{input: "1y4m-5d-3h", d: Duration{Years: 1, Months: 4, Days: -5, Hours: -3}, output: "1y4m-5d-3h"},
		{input: "2y7m-5d", d: Duration{Years: 2, Months: 7, Days: -5}, output: "2y7m-5d"},
		{input: "2w", err: true},
		{input: "1y4m3w1d", err: true},
		{input: "s", err: true},
		{input: "\xdf\x80", err: true}, // NKO DIGIT ZERO; we want ASCII digits
	}

	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			d, err := ParseDuration(test.input)
			if test.err {
				if err == nil {
					t.Fatalf("Missing error for %v", test.input)
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
			}

			if !cmp.Equal(d, test.d) {
				t.Error(cmp.Diff(test.d, d))
			}

			s := d.String()
			if s != test.output {
				t.Errorf("unexpected return of String(), want %q, got %q", test.output, s)
			}
		})
	}
}

func TestDurationTimePattern(t *testing.T) {
	// duration as string and equivalent number of hours
	type TimeOffsetResult struct {
		duration      string
		durationHours int
	}

	referenceTime := DurationTime{}
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
		temp := DurationTime{}
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

	referenceTime := DurationTime{}
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
	timeDurations := make([]DurationTime, 0, len(timeOffsets))
	for i, offset := range timeOffsets {
		temp := DurationTime{}
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
