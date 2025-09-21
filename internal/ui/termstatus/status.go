package termstatus

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/restic/restic/internal/terminal"
	"github.com/restic/restic/internal/ui"
)

var _ ui.Terminal = &Terminal{}

// Terminal is used to write messages and display status lines which can be
// updated. When the output is redirected to a file, the status lines are not
// printed.
type Terminal struct {
	wr              io.Writer
	fd              uintptr
	errWriter       io.Writer
	msg             chan message
	status          chan status
	canUpdateStatus bool
	lastStatusLen   int

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
		wr:        wr,
		errWriter: errWriter,
		msg:       make(chan message),
		status:    make(chan status),
		closed:    make(chan struct{}),
	}

	if disableStatus {
		return t
	}

	if d, ok := wr.(fder); ok && terminal.CanUpdateStatus(d.Fd()) {
		// only use the fancy status code when we're running on a real terminal.
		t.canUpdateStatus = true
		t.fd = d.Fd()
		t.clearCurrentLine = terminal.ClearCurrentLine(t.fd)
		t.moveCursorUp = terminal.MoveCursorUp(t.fd)
	}

	return t
}

// CanUpdateStatus return whether the status output is updated in place.
func (t *Terminal) CanUpdateStatus() bool {
	return t.canUpdateStatus
}

// OutputRaw returns the output writer. Should only be used if there is no
// other option. Must not be used in combination with Print, Error, SetStatus
// or any other method that writes to the terminal.
func (t *Terminal) OutputRaw() io.Writer {
	return t.wr
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

// run listens on the channels and updates the terminal screen.
func (t *Terminal) run(ctx context.Context) {
	var status []string
	for {
		select {
		case <-ctx.Done():
			if !terminal.IsProcessBackground(t.fd) {
				t.writeStatus([]string{})
			}

			return

		case msg := <-t.msg:
			if terminal.IsProcessBackground(t.fd) {
				// ignore all messages, do nothing, we are in the background process group
				continue
			}
			t.clearCurrentLine(t.wr, t.fd)

			var dst io.Writer
			if msg.err {
				dst = t.errWriter
			} else {
				dst = t.wr
			}

			if _, err := io.WriteString(dst, msg.line); err != nil {
				fmt.Fprintf(os.Stderr, "write failed: %v\n", err)
				continue
			}

			t.writeStatus(status)
		case stat := <-t.status:
			status = append(status[:0], stat.lines...)

			if terminal.IsProcessBackground(t.fd) {
				// ignore all messages, do nothing, we are in the background process group
				continue
			}

			t.writeStatus(status)
		}
	}
}

func (t *Terminal) writeStatus(status []string) {
	statusLen := len(status)
	status = append([]string{}, status...)
	for i := len(status); i < t.lastStatusLen; i++ {
		// clear no longer used status lines
		status = append(status, "")
		if i > 0 {
			// all lines except the last one must have a line break
			status[i-1] = status[i-1] + "\n"
		}
	}
	t.lastStatusLen = statusLen

	for _, line := range status {
		t.clearCurrentLine(t.wr, t.fd)

		_, err := t.wr.Write([]byte(line))
		if err != nil {
			fmt.Fprintf(os.Stderr, "write failed: %v\n", err)
		}
	}

	if len(status) > 0 {
		t.moveCursorUp(t.wr, t.fd, len(status)-1)
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

			var dst io.Writer
			if msg.err {
				dst = t.errWriter
			} else {
				dst = t.wr
			}

			if _, err := io.WriteString(dst, msg.line); err != nil {
				_, _ = fmt.Fprintf(os.Stderr, "write failed: %v\n", err)
			}

		case stat := <-t.status:
			for _, line := range stat.lines {
				// Ensure that each message ends with exactly one newline.
				if _, err := fmt.Fprintln(t.wr, strings.TrimRight(line, "\n")); err != nil {
					_, _ = fmt.Fprintf(os.Stderr, "write failed: %v\n", err)
				}
			}
		}
	}
}

func (t *Terminal) print(line string, isErr bool) {
	// make sure the line ends with a line break
	if len(line) == 0 || line[len(line)-1] != '\n' {
		line += "\n"
	}

	select {
	case t.msg <- message{line: line, err: isErr}:
	case <-t.closed:
	}
}

// Print writes a line to the terminal.
func (t *Terminal) Print(line string) {
	t.print(line, false)
}

// Error writes an error to the terminal.
func (t *Terminal) Error(line string) {
	t.print(line, true)
}

func sanitizeLines(lines []string, width int) []string {
	// Sanitize lines and truncate them if they're too long.
	for i, line := range lines {
		line = ui.Quote(line)
		if width > 0 {
			line = ui.Truncate(line, width-2)
		}
		if i < len(lines)-1 { // Last line gets no line break.
			line += "\n"
		}
		lines[i] = line
	}
	return lines
}

// SetStatus updates the status lines.
// The lines should not contain newlines; this method adds them.
// Pass nil or an empty array to remove the status lines.
func (t *Terminal) SetStatus(lines []string) {
	// only truncate interactive status output
	var width int
	if t.canUpdateStatus {
		width = terminal.Width(t.fd)
		if width <= 0 {
			// use 80 columns by default
			width = 80
		}
	}

	sanitizeLines(lines, width)

	select {
	case t.status <- status{lines: lines}:
	case <-t.closed:
	}
}
