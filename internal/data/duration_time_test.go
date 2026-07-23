package data

import (
	"fmt"
	"testing"

	"github.com/restic/restic/internal/test"
)

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
			"test %d: expected %q, but got %q", i, tt, str)
	}

	timeStamp := referenceTime.GetTime()
	for i, elem := range timeDurations {
		asTime := elem.GetTime()
		diff := timeStamp.Sub(asTime).Hours()
		test.Assert(t, diff == float64(timeOffsets[i].durationHours),
			"test %d: expected %f hours difference, but got %f hours difference",
			i, float64(timeOffsets[i].durationHours), diff)
	}
}
