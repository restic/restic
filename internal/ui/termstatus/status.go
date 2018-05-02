package termstatus

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
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
	clearLines      clearLinesFunc
}

type clearLinesFunc func(wr io.Writer, fd uintptr, n int)

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
	}

	if disableStatus {
		return t
	}

	if d, ok := wr.(fder); ok && canUpdateStatus(d.Fd()) {
		// only use the fancy status code when we're running on a real terminal.
		t.canUpdateStatus = true
		t.fd = d.Fd()
		t.clearLines = clearLines(wr, t.fd)
	}

	return t
}

// Run updates the screen. It should be run in a separate goroutine. When
// ctx is cancelled, the status lines are cleanly removed.
func (t *Terminal) Run(ctx context.Context) {
	if t.canUpdateStatus {
		t.run(ctx)
		return
	}

	t.runWithoutStatus(ctx)
}

func countLines(buf []byte) int {
	lines := 0
	sc := bufio.NewScanner(bytes.NewReader(buf))
	for sc.Scan() {
		lines++
	}
	return lines
}

type stringWriter interface {
	WriteString(string) (int, error)
}

// run listens on the channels and updates the terminal screen.
func (t *Terminal) run(ctx context.Context) {
	statusBuf := bytes.NewBuffer(nil)
	statusLines := 0
	for {
		select {
		case <-ctx.Done():
			if IsProcessBackground() {
				// ignore all messages, do nothing, we are in the background process group
				continue
			}
			t.undoStatus(statusLines)

			err := t.wr.Flush()
			if err != nil {
				fmt.Fprintf(os.Stderr, "flush failed: %v\n", err)
			}

			return

		case msg := <-t.msg:
			if IsProcessBackground() {
				// ignore all messages, do nothing, we are in the background process group
				continue
			}
			t.undoStatus(statusLines)

			var dst io.Writer
			if msg.err {
				dst = t.errWriter

				// assume t.wr and t.errWriter are different, so we need to
				// flush the removal of the status lines first.
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

			_, err = t.wr.Write(statusBuf.Bytes())
			if err != nil {
				fmt.Fprintf(os.Stderr, "write failed: %v\n", err)
			}

			err = t.wr.Flush()
			if err != nil {
				fmt.Fprintf(os.Stderr, "flush failed: %v\n", err)
			}

		case stat := <-t.status:
			if IsProcessBackground() {
				// ignore all messages, do nothing, we are in the background process group
				continue
			}
			t.undoStatus(statusLines)

			statusBuf.Reset()
			for _, line := range stat.lines {
				statusBuf.WriteString(line)
			}
			statusLines = len(stat.lines)

			_, err := t.wr.Write(statusBuf.Bytes())
			if err != nil {
				fmt.Fprintf(os.Stderr, "write failed: %v\n", err)
			}

			err = t.wr.Flush()
			if err != nil {
				fmt.Fprintf(os.Stderr, "flush failed: %v\n", err)
			}
		}
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
	if lines == 0 {
		return
	}

	lines--
	t.clearLines(t.wr, t.fd, lines)
}

// Print writes a line to the terminal.
func (t *Terminal) Print(line string) {
	// make sure the line ends with a line break
	if line[len(line)-1] != '\n' {
		line += "\n"
	}

	t.msg <- message{line: line}
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

	t.msg <- message{line: line, err: true}
}

// Errorf uses fmt.Sprintf to write an error line to the terminal.
func (t *Terminal) Errorf(msg string, args ...interface{}) {
	s := fmt.Sprintf(msg, args...)
	t.Error(s)
}

// SetStatus updates the status lines.
func (t *Terminal) SetStatus(lines []string) {
	if len(lines) == 0 {
		return
	}

	width, _, err := getTermSize(t.fd)
	if err != nil || width < 0 {
		// use 80 columns by default
		width = 80
	}

	// make sure that all lines have a line break and are not too long
	for i, line := range lines {
		line = strings.TrimRight(line, "\n")

		if len(line) >= width-2 {
			line = line[:width-2]
		}
		line += "\n"
		lines[i] = line
	}

	// make sure the last line does not have a line break
	last := len(lines) - 1
	lines[last] = strings.TrimRight(lines[last], "\n")

	t.status <- status{lines: lines}
}
