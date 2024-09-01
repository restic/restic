package termstatus

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strconv"
	"testing"

	rtest "github.com/restic/restic/internal/test"
)

func TestSetStatus(t *testing.T) {
	var buf bytes.Buffer
	term := New(&buf, io.Discard, false)

	term.canUpdateStatus = true
	term.fd = ^uintptr(0)
	term.clearCurrentLine = posixClearCurrentLine
	term.moveCursorUp = posixMoveCursorUp

	ctx, cancel := context.WithCancel(context.Background())
	go term.Run(ctx)

	const (
		clear = posixControlClearLine
		home  = posixControlMoveCursorHome
		up    = posixControlMoveCursorUp
	)

	term.SetStatus([]string{"first"})
	exp := home + clear + "first" + home

	term.SetStatus([]string{""})
	exp += home + clear + "" + home

	term.SetStatus([]string{})
	exp += home + clear + "" + home

	// already empty status
	term.SetStatus([]string{})

	term.SetStatus([]string{"foo", "bar", "baz"})
	exp += home + clear + "foo\n" + home + clear + "bar\n" +
		home + clear + "baz" + home + up + up

	term.SetStatus([]string{"quux", "needs\nquote"})
	exp += home + clear + "quux\n" +
		home + clear + "\"needs\\nquote\"\n" +
		home + clear + home + up + up // Clear third line

	cancel()
	exp += home + clear + "\n" + home + clear + home + up // Status cleared

	<-term.closed
	rtest.Equals(t, exp, buf.String())
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

func TestSanitizeLines(t *testing.T) {
	var tests = []struct {
		input  []string
		width  int
		output []string
	}{
		{[]string{""}, 80, []string{""}},
		{[]string{"too long test line"}, 10, []string{"too long"}},
		{[]string{"too long test line", "text"}, 10, []string{"too long\n", "text"}},
		{[]string{"too long test line", "second long test line"}, 10, []string{"too long\n", "second l"}},
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("%s %d", test.input, test.width), func(t *testing.T) {
			out := sanitizeLines(test.input, test.width)
			rtest.Equals(t, test.output, out)
		})
	}
}
