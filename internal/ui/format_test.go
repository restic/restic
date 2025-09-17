package ui

import (
	"strconv"
	"testing"

	rtest "github.com/restic/restic/internal/test"
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
		rtest.OK(t, err)
		rtest.Equals(t, tt.expected, actual)
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
		rtest.Equals(t, int64(0), v)
	}
}

func TestDisplayWidth(t *testing.T) {
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
		if got := DisplayWidth(c.input); got != c.want {
			t.Errorf("wrong display width for '%s', want %d, got %d", c.input, c.want, got)
		}
	}

}

func TestQuote(t *testing.T) {
	for _, c := range []struct {
		in        string
		needQuote bool
	}{
		{"foo.bar/baz", false},
		{"föó_bàŕ-bãẑ", false},
		{" foo ", false},
		{"foo bar", false},
		{"foo\nbar", true},
		{"foo\rbar", true},
		{"foo\abar", true},
		{"\xff", true},
		{`c:\foo\bar`, false},
		// Issue #2260: terminal control characters.
		{"\x1bm_red_is_beautiful", true},
	} {
		if c.needQuote {
			rtest.Equals(t, strconv.Quote(c.in), Quote(c.in))
		} else {
			rtest.Equals(t, c.in, Quote(c.in))
		}
	}
}

func TestTruncate(t *testing.T) {
	var tests = []struct {
		input  string
		width  int
		output string
	}{
		{"", 80, ""},
		{"", 0, ""},
		{"", -1, ""},
		{"foo", 80, "foo"},
		{"foo", 4, "foo"},
		{"foo", 3, "foo"},
		{"foo", 2, "fo"},
		{"foo", 1, "f"},
		{"foo", 0, ""},
		{"foo", -1, ""},
		{"Löwen", 4, "Löwe"},
		{"あああああ/data", 7, "あああ"},
		{"あああああ/data", 10, "あああああ"},
		{"あああああ/data", 11, "あああああ/"},
	}

	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			out := Truncate(test.input, test.width)
			if out != test.output {
				t.Fatalf("wrong output for input %v, width %d: want %q, got %q",
					test.input, test.width, test.output, out)
			}
		})
	}
}

func benchmarkTruncate(b *testing.B, s string, w int) {
	for i := 0; i < b.N; i++ {
		Truncate(s, w)
	}
}

func BenchmarkTruncateASCII(b *testing.B) {
	s := "This is an ASCII-only status message...\r\n"
	benchmarkTruncate(b, s, len(s)-1)
}

func BenchmarkTruncateUnicode(b *testing.B) {
	s := "Hello World or Καλημέρα κόσμε or こんにちは 世界"
	w := 0
	for i := 0; i < len(s); {
		w++
		wide, utfsize := wideRune(s[i:])
		if wide {
			w++
		}
		i += int(utfsize)
	}
	b.ResetTimer()

	benchmarkTruncate(b, s, w-1)
}
