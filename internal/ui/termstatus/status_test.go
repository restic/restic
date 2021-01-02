package termstatus

import "testing"

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
		{"あああああああああ/data", 10, "あああああ"},
		{"あああああああああ/data", 11, "あああああ"},
	}

	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			out := truncate(test.input, test.width)
			if out != test.output {
				t.Fatalf("wrong output for input %v, width %d: want %q, got %q",
					test.input, test.width, test.output, out)
			}
		})
	}
}
