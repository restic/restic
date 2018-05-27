package termstatus

import "testing"

func TestTruncate(t *testing.T) {
	var tests = []struct {
		input  string
		maxlen int
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
	}

	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			out := truncate(test.input, test.maxlen)
			if out != test.output {
				t.Fatalf("wrong output for input %v, maxlen %d: want %q, got %q",
					test.input, test.maxlen, test.output, out)
			}
		})
	}
}
