package ui

import (
	"testing"

	"github.com/restic/restic/internal/test"
)

func TestFormatBytes(t *testing.T) {
	for _, c := range []struct {
		size uint64
		want string
	}{
		{0, "0 B"},
		{1023, "1023 B"},
		{1024, "1.000 KiB"},
		{5<<20 + 1<<19, "5.500 MiB"},
		{1 << 30, "1.000 GiB"},
		{2 << 30, "2.000 GiB"},
		{1<<40 - 1<<36, "960.000 GiB"},
		{1 << 40, "1.000 TiB"},
	} {
		if got := FormatBytes(c.size); got != c.want {
			t.Errorf("want %q, got %q", c.want, got)
		}
	}
}

func TestFormatPercent(t *testing.T) {
	for _, c := range []struct {
		num, denom uint64
		want       string
	}{
		{0, 5, "0.00%"},
		{3, 7, "42.86%"},
		{99, 99, "100.00%"},
	} {
		if got := FormatPercent(c.num, c.denom); got != c.want {
			t.Errorf("want %q, got %q", c.want, got)
		}
	}
}

func TestParseBytes(t *testing.T) {
	for _, tt := range []struct {
		in       string
		expected int64
	}{
		{"1024", 1024},
		{"1024b", 1024},
		{"1024B", 1024},
		{"1k", 1024},
		{"100k", 102400},
		{"100K", 102400},
		{"10M", 10485760},
		{"100m", 104857600},
		{"20G", 21474836480},
		{"10g", 10737418240},
		{"2T", 2199023255552},
		{"2t", 2199023255552},
		{"9223372036854775807", 1<<63 - 1},
	} {
		actual, err := ParseBytes(tt.in)
		test.OK(t, err)
		test.Equals(t, tt.expected, actual)
	}
}

func TestParseBytesInvalid(t *testing.T) {
	for _, s := range []string{
		"",
		" ",
		"foobar",
		"zzz",
		"18446744073709551615", // 1<<64-1.
		"9223372036854775807k", // 1<<63-1 kiB.
		"9999999999999M",
		"99999999999999999999",
	} {
		v, err := ParseBytes(s)
		if err == nil {
			t.Errorf("wanted error for invalid value %q, got nil", s)
		}
		test.Equals(t, int64(0), v)
	}
}

func TestTerminalDisplayWidth(t *testing.T) {
	for _, c := range []struct {
		input string
		want  int
	}{
		{"foo", 3},
		{"aéb", 3},
		{"ab", 3},
		{"a’b", 3},
		{"aあb", 4},
	} {
		if got := TerminalDisplayWidth(c.input); got != c.want {
			t.Errorf("wrong display width for '%s', want %d, got %d", c.input, c.want, got)
		}
	}

}
