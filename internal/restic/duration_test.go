package restic

import (
	"testing"

	"github.com/google/go-cmp/cmp"
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
	}{
		{"9h", Duration{Hours: 9}, "9h"},
		{"3d", Duration{Days: 3}, "3d"},
		{"4d2h", Duration{Days: 4, Hours: 2}, "4d2h"},
		{"7m5d", Duration{Months: 7, Days: 5}, "7m5d"},
		{"6m4d8h", Duration{Months: 6, Days: 4, Hours: 8}, "6m4d8h"},
		{"5d7m", Duration{Months: 7, Days: 5}, "7m5d"},
		{"4h3d9m", Duration{Months: 9, Days: 3, Hours: 4}, "9m3d4h"},
		{"-7m5d", Duration{Months: -7, Days: 5}, "-7m5d"},
		{"1y4m-5d-3h", Duration{Years: 1, Months: 4, Days: -5, Hours: -3}, "1y4m-5d-3h"},
		{"2y7m-5d", Duration{Years: 2, Months: 7, Days: -5}, "2y7m-5d"},
	}

	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			d, err := ParseDuration(test.input)
			if err != nil {
				t.Fatal(err)
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
