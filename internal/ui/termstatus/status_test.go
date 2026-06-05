package termstatus

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/restic/restic/internal/terminal"
	rtest "github.com/restic/restic/internal/test"
)

func TestSetStatus(t *testing.T) {
	buf, term, cancel := setupStatusTest()

	const (
		cl   = terminal.PosixControlClearLine
		home = terminal.PosixControlMoveCursorHome
		up   = terminal.PosixControlMoveCursorUp

		clearLn = home + cl
	)

	term.SetStatus([]string{"first"})
	exp := clearLn + "first" + home

	term.SetStatus([]string{""})
	exp += clearLn + "" + home

	term.SetStatus([]string{})
	exp += clearLn + "" + home

	// already empty status
	term.SetStatus([]string{})

	term.SetStatus([]string{"foo", "bar", "baz"})
	exp += clearLn + "foo\n" + clearLn + "bar\n" + clearLn + "baz" + home + up + up

	term.SetStatus([]string{"quux", "needs\nquote"})
	exp += clearLn + "quux\n" +
		clearLn + "\"needs\\nquote\"\n" +
		clearLn + home + up + up // Clear third line

	cancel()
	exp += clearLn + "\n" + clearLn + "" + home + up // Status cleared

	<-term.closed
	rtest.Equals(t, exp, buf.String())
}

func TestSetStatusUnchangedLines(t *testing.T) {
	buf, term, cancel := setupStatusTest()

	const (
		cl   = terminal.PosixControlClearLine
		home = terminal.PosixControlMoveCursorHome
		up   = terminal.PosixControlMoveCursorUp
		down = terminal.PosixControlMoveCursorDown

		clearLn  = home + cl
		stepDown = home + down
	)

	term.SetStatus([]string{"line1", "line2", "line3"})
	exp := clearLn + "line1\n" + clearLn + "line2\n" + clearLn + "line3" + home + up + up

	term.SetStatus([]string{"line1", "line2", "line3-changed"})
	exp += stepDown + stepDown + clearLn + "line3-changed" + home + up + up

	term.SetStatus([]string{"line1", "line2", "line3-changed"})

	term.SetStatus([]string{"line1", "line2-new", "line3-changed"})
	exp += stepDown + clearLn + "line2-new\n" + home + up + up

	cancel()
	exp += clearLn + "\n" + clearLn + "\n" + clearLn + "" + home + up + up

	<-term.closed
	rtest.Equals(t, exp, buf.String())
}

func setupStatusTest() (*bytes.Buffer, *Terminal, context.CancelFunc) {
	buf := &bytes.Buffer{}
	term := new(nil, buf, buf, false)

	term.canUpdateStatus = true
	term.fd = ^uintptr(0)
	term.clearCurrentLine = terminal.PosixClearCurrentLine
	term.moveCursorUp = terminal.PosixMoveCursorUp
	term.moveCursorDown = terminal.PosixMoveCursorDown

	ctx, cancel := context.WithCancel(context.Background())
	go term.Run(ctx)
	return buf, term, cancel
}

func TestPrint(t *testing.T) {
	buf, term, cancel := setupStatusTest()

	const (
		cl   = terminal.PosixControlClearLine
		home = terminal.PosixControlMoveCursorHome
	)

	term.Print("test")
	exp := home + cl + "test\n"
	term.Error("error")
	exp += home + cl + "error\n"

	cancel()

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
		{[]string{"too long test line", "text"}, 10, []string{"too long", "text"}},
		{[]string{"too long test line", "second long test line"}, 10, []string{"too long", "second l"}},
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("%s %d", test.input, test.width), func(t *testing.T) {
			out := sanitizeLines(test.input, test.width)
			rtest.Equals(t, test.output, out)
		})
	}
}

type errorReader struct{ err error }

func (r *errorReader) Read([]byte) (int, error) { return 0, r.err }

func TestReadPassword(t *testing.T) {
	want := errors.New("foo")
	_, err := readPassword(&errorReader{want})
	rtest.Assert(t, errors.Is(err, want), "wrong error %v", err)
}

func TestReadPasswordTerminal(t *testing.T) {
	expected := "password"
	term := new(io.NopCloser(strings.NewReader(expected)), io.Discard, io.Discard, false)
	pw, err := term.ReadPassword(context.Background(), "test")
	rtest.OK(t, err)
	rtest.Equals(t, expected, pw)
}

func TestRawInputOutput(t *testing.T) {
	input := io.NopCloser(strings.NewReader("password"))
	var output bytes.Buffer
	term, cancel := Setup(input, &output, io.Discard, false)
	defer cancel()
	rtest.Equals(t, input, term.InputRaw())
	rtest.Equals(t, false, term.InputIsTerminal())
	rtest.Equals(t, io.Writer(&output), term.OutputRaw())
	rtest.Equals(t, false, term.OutputIsTerminal())
	rtest.Equals(t, false, term.CanUpdateStatus())
}

func TestDisableStatus(t *testing.T) {
	var output bytes.Buffer
	term, cancel := Setup(nil, &output, &output, true)
	rtest.Equals(t, false, term.CanUpdateStatus())

	term.Print("test")
	term.Error("error")
	term.SetStatus([]string{"status"})

	cancel()
	rtest.Equals(t, "test\nerror\nstatus\n", output.String())
}

func TestOutputWriter(t *testing.T) {
	var output bytes.Buffer
	term, cancel := Setup(nil, &output, &output, true)

	_, err := term.OutputWriter().Write([]byte("output\npartial"))
	rtest.OK(t, err)
	term.Print("test")
	term.Error("error")

	cancel()
	rtest.Equals(t, "output\ntest\nerror\npartial\n", output.String())
}
