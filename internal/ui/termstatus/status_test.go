package termstatus

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"testing"

	"github.com/restic/restic/internal/terminal"
	rtest "github.com/restic/restic/internal/test"
)

func TestSetStatus(t *testing.T) {
	var buf bytes.Buffer
	term := New(&buf, io.Discard, false)

	term.canUpdateStatus = true
	term.fd = ^uintptr(0)
	term.clearCurrentLine = terminal.PosixClearCurrentLine
	term.moveCursorUp = terminal.PosixMoveCursorUp

	ctx, cancel := context.WithCancel(context.Background())
	go term.Run(ctx)

	const (
		cl   = terminal.PosixControlClearLine
		home = terminal.PosixControlMoveCursorHome
		up   = terminal.PosixControlMoveCursorUp
	)

	term.SetStatus([]string{"first"})
	exp := home + cl + "first" + home

	term.SetStatus([]string{""})
	exp += home + cl + "" + home

	term.SetStatus([]string{})
	exp += home + cl + "" + home

	// already empty status
	term.SetStatus([]string{})

	term.SetStatus([]string{"foo", "bar", "baz"})
	exp += home + cl + "foo\n" + home + cl + "bar\n" +
		home + cl + "baz" + home + up + up

	term.SetStatus([]string{"quux", "needs\nquote"})
	exp += home + cl + "quux\n" +
		home + cl + "\"needs\\nquote\"\n" +
		home + cl + home + up + up // Clear third line

	cancel()
	exp += home + cl + "\n" + home + cl + home + up // Status cleared

	<-term.closed
	rtest.Equals(t, exp, buf.String())
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
