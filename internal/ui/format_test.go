package ui

import "testing"

func TestFormatBytes(t *testing.T) {
	for _, c := range []struct {
		size uint64
		want string
	}{
		{0, "0 B"},
		{1025, "1.001 KiB"},
		{1<<30 + 7, "1.000 GiB"},
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
