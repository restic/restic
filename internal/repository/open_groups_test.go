package repository

import "testing"

func TestClampOpenTreeGroups(t *testing.T) {
	for _, tc := range []struct {
		in, want int
	}{
		{-100, minOpenTreeGroups},
		{0, minOpenTreeGroups},
		{minOpenTreeGroups - 1, minOpenTreeGroups},
		{minOpenTreeGroups, minOpenTreeGroups},
		{500, 500},
		{maxOpenTreeGroupsCap, maxOpenTreeGroupsCap},
		{maxOpenTreeGroupsCap + 1, maxOpenTreeGroupsCap},
		{1 << 30, maxOpenTreeGroupsCap},
	} {
		if got := clampOpenTreeGroups(tc.in); got != tc.want {
			t.Errorf("clampOpenTreeGroups(%d) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestMaxOpenTreeGroups(t *testing.T) {
	// whatever the platform / limit, the result must be within the clamp range
	n := MaxOpenTreeGroups()
	if n < minOpenTreeGroups || n > maxOpenTreeGroupsCap {
		t.Errorf("MaxOpenTreeGroups() = %d, want within [%d, %d]", n, minOpenTreeGroups, maxOpenTreeGroupsCap)
	}
}
