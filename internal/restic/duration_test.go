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
