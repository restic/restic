package ui

import "testing"

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
