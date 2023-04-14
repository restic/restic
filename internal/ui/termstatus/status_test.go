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
