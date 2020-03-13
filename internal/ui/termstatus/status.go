package termstatus

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/crypto/ssh/terminal"
)

// Terminal is used to write messages and display status lines which can be
// updated. When the output is redirected to a file, the status lines are not
// printed.
type Terminal struct {
	wr              *bufio.Writer
	fd              uintptr
	errWriter       io.Writer
	buf             *bytes.Buffer
	msg             chan message
	status          chan status
	canUpdateStatus bool

	// will be closed when the goroutine which runs Run() terminates, so it'll
	// yield a default value immediately
	closed chan struct{}

	clearCurrentLine func(io.Writer, uintptr)
	moveCursorUp     func(io.Writer, uintptr, int)
}

type message struct {
	line string
	err  bool
}

type status struct {
	lines []string
}

type fder interface {
	Fd() uintptr
}

// New returns a new Terminal for wr. A goroutine is started to update the
// terminal. It is terminated when ctx is cancelled. When wr is redirected to
// a file (e.g. via shell output redirection) or is just an io.Writer (not the
// open *os.File for stdout), no status lines are printed. The status lines and
// normal output (via Print/Printf) are written to wr, error messages are
// written to errWriter. If disableStatus is set to true, no status messages
// are printed even if the terminal supports it.
func New(wr io.Writer, errWriter io.Writer, disableStatus bool) *Terminal {
	t := &Terminal{
		wr:        bufio.NewWriter(wr),
		errWriter: errWriter,
		buf:       bytes.NewBuffer(nil),
		msg:       make(chan message),
		status:    make(chan status),
		closed:    make(chan struct{}),
	}

	if disableStatus {
		return t
	}

	if d, ok := wr.(fder); ok && canUpdateStatus(d.Fd()) {
		// only use the fancy status code when we're running on a real terminal.
		t.canUpdateStatus = true
		t.fd = d.Fd()
		t.clearCurrentLine = clearCurrentLine(wr, t.fd)
		t.moveCursorUp = moveCursorUp(wr, t.fd)
	}

	return t
}

// Run updates the screen. It should be run in a separate goroutine. When
// ctx is cancelled, the status lines are cleanly removed.
func (t *Terminal) Run(ctx context.Context) {
	defer close(t.closed)
	if t.canUpdateStatus {
		t.run(ctx)
		return
	}

	t.runWithoutStatus(ctx)
}

type stringWriter interface {
	WriteString(string) (int, error)
}

// run listens on the channels and updates the terminal screen.
func (t *Terminal) run(ctx context.Context) {
	var status []string
	for {
		select {
		case <-ctx.Done():
			if IsProcessBackground() {
				// ignore all messages, do nothing, we are in the background process group
				continue
			}
			t.undoStatus(len(status))

			return

		case msg := <-t.msg:
			if IsProcessBackground() {
				// ignore all messages, do nothing, we are in the background process group
				continue
			}
			t.clearCurrentLine(t.wr, t.fd)

			var dst io.Writer
			if msg.err {
				dst = t.errWriter

				// assume t.wr and t.errWriter are different, so we need to
				// flush clearing the current line
				err := t.wr.Flush()
				if err != nil {
					fmt.Fprintf(os.Stderr, "flush failed: %v\n", err)
				}
			} else {
				dst = t.wr
			}

			var err error
			if w, ok := dst.(stringWriter); ok {
				_, err = w.WriteString(msg.line)
			} else {
				_, err = dst.Write([]byte(msg.line))
			}

			if err != nil {
				fmt.Fprintf(os.Stderr, "write failed: %v\n", err)
				continue
			}

			t.writeStatus(status)

			err = t.wr.Flush()
			if err != nil {
				fmt.Fprintf(os.Stderr, "flush failed: %v\n", err)
			}

		case stat := <-t.status:
			if IsProcessBackground() {
				// ignore all messages, do nothing, we are in the background process group
				continue
			}

			status = status[:0]
			status = append(status, stat.lines...)
			t.writeStatus(status)
		}
	}
}

func (t *Terminal) writeStatus(status []string) {
	for _, line := range status {
		t.clearCurrentLine(t.wr, t.fd)

		_, err := t.wr.WriteString(line)
		if err != nil {
			fmt.Fprintf(os.Stderr, "write failed: %v\n", err)
		}

		// flush is needed so that the current line is updated
		err = t.wr.Flush()
		if err != nil {
			fmt.Fprintf(os.Stderr, "flush failed: %v\n", err)
		}
	}

	if len(status) > 0 {
		t.moveCursorUp(t.wr, t.fd, len(status)-1)
	}

	err := t.wr.Flush()
	if err != nil {
		fmt.Fprintf(os.Stderr, "flush failed: %v\n", err)
	}
}

// runWithoutStatus listens on the channels and just prints out the messages,
// without status lines.
func (t *Terminal) runWithoutStatus(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-t.msg:
			var err error
			var flush func() error

			var dst io.Writer
			if msg.err {
				dst = t.errWriter
			} else {
				dst = t.wr
				flush = t.wr.Flush
			}

			if w, ok := dst.(stringWriter); ok {
				_, err = w.WriteString(msg.line)
			} else {
				_, err = dst.Write([]byte(msg.line))
			}

			if err != nil {
				fmt.Fprintf(os.Stderr, "write failed: %v\n", err)
			}

			if flush == nil {
				continue
			}

			err = flush()
			if err != nil {
				fmt.Fprintf(os.Stderr, "flush failed: %v\n", err)
			}

		case _ = <-t.status:
			// discard status lines
		}
	}
}

func (t *Terminal) undoStatus(lines int) {
	for i := 0; i < lines; i++ {
		t.clearCurrentLine(t.wr, t.fd)

		_, err := t.wr.WriteRune('\n')
		if err != nil {
			fmt.Fprintf(os.Stderr, "write failed: %v\n", err)
		}

		// flush is needed so that the current line is updated
		err = t.wr.Flush()
		if err != nil {
			fmt.Fprintf(os.Stderr, "flush failed: %v\n", err)
		}
	}

	t.moveCursorUp(t.wr, t.fd, lines)

	err := t.wr.Flush()
	if err != nil {
		fmt.Fprintf(os.Stderr, "flush failed: %v\n", err)
	}
}

// Print writes a line to the terminal.
func (t *Terminal) Print(line string) {
	// make sure the line ends with a line break
	if line[len(line)-1] != '\n' {
		line += "\n"
	}

	select {
	case t.msg <- message{line: line}:
	case <-t.closed:
	}
}

// Printf uses fmt.Sprintf to write a line to the terminal.
func (t *Terminal) Printf(msg string, args ...interface{}) {
	s := fmt.Sprintf(msg, args...)
	t.Print(s)
}

// Error writes an error to the terminal.
func (t *Terminal) Error(line string) {
	// make sure the line ends with a line break
	if line[len(line)-1] != '\n' {
		line += "\n"
	}

	select {
	case t.msg <- message{line: line, err: true}:
	case <-t.closed:
	}
}

// Errorf uses fmt.Sprintf to write an error line to the terminal.
func (t *Terminal) Errorf(msg string, args ...interface{}) {
	s := fmt.Sprintf(msg, args...)
	t.Error(s)
}

// truncate returns a string that has at most maxlen characters. If maxlen is
// negative, the empty string is returned.
func truncate(s string, maxlen int) string {
	if maxlen < 0 {
		return ""
	}

	if len(s) < maxlen {
		return s
	}

	return s[:maxlen]
}

// SetStatus updates the status lines.
func (t *Terminal) SetStatus(lines []string) {
	if len(lines) == 0 {
		return
	}

	width, _, err := terminal.GetSize(int(t.fd))
	if err != nil || width <= 0 {
		// use 80 columns by default
		width = 80
	}

	// make sure that all lines have a line break and are not too long
	for i, line := range lines {
		line = strings.TrimRight(line, "\n")
		line = truncate(line, width-2) + "\n"
		lines[i] = line
	}

	// make sure the last line does not have a line break
	last := len(lines) - 1
	lines[last] = strings.TrimRight(lines[last], "\n")

	select {
	case t.status <- status{lines: lines}:
	case <-t.closed:
	}
}
