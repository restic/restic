package termstatus

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/restic/restic/internal/terminal"
	"github.com/restic/restic/internal/ui"
)

var _ ui.Terminal = &Terminal{}

// Terminal is used to write messages and display status lines which can be
// updated. When the output is redirected to a file, the status lines are not
// printed.
type Terminal struct {
	rd               io.ReadCloser
	inFd             uintptr
	wr               io.Writer
	fd               uintptr
	errWriter        io.Writer
	msg              chan message
	status           chan status
	lastStatusLen    int
	inputIsTerminal  bool
	outputIsTerminal bool
	canUpdateStatus  bool

	outputWriter     io.WriteCloser
	outputWriterOnce sync.Once

	// will be closed when the goroutine which runs Run() terminates, so it'll
	// yield a default value immediately
	closed chan struct{}

	clearCurrentLine func(io.Writer, uintptr) error
	moveCursorUp     func(io.Writer, uintptr, int) error
}

type message struct {
	line    string
	err     bool
	barrier chan struct{}
}

type status struct {
	lines []string
}

type fder interface {
	Fd() uintptr
}

// Setup creates a new termstatus.
// The returned function must be called to shut down the termstatus,
//
// Expected usage:
// ```
// term, cancel := termstatus.Setup(os.Stdin, os.Stdout, os.Stderr, false)
// defer cancel()
// // do stuff
// ```
func Setup(stdin io.ReadCloser, stdout, stderr io.Writer, quiet bool) (*Terminal, func()) {
	var wg sync.WaitGroup
	// only shutdown once cancel is called to ensure that no output is lost
	cancelCtx, cancel := context.WithCancel(context.Background())

	term := New(stdin, stdout, stderr, quiet)
	wg.Add(1)
	go func() {
		defer wg.Done()
		term.Run(cancelCtx)
	}()

	return term, func() {
		if term.outputWriter != nil {
			_ = term.outputWriter.Close()
		}
		term.Flush()
		// shutdown termstatus
		cancel()
		wg.Wait()
	}
}

// New returns a new Terminal for wr. A goroutine is started to update the
// terminal. It is terminated when ctx is cancelled. When wr is redirected to
// a file (e.g. via shell output redirection) or is just an io.Writer (not the
// open *os.File for stdout), no status lines are printed. The status lines and
// normal output (via Print/Printf) are written to wr, error messages are
// written to errWriter. If disableStatus is set to true, no status messages
// are printed even if the terminal supports it.
func New(rd io.ReadCloser, wr io.Writer, errWriter io.Writer, disableStatus bool) *Terminal {
	t := &Terminal{
		rd:        rd,
		wr:        wr,
		errWriter: errWriter,
		msg:       make(chan message),
		status:    make(chan status),
		closed:    make(chan struct{}),
	}

	if disableStatus {
		return t
	}

	if d, ok := rd.(fder); ok {
		if terminal.InputIsTerminal(d.Fd()) {
			t.inFd = d.Fd()
			t.inputIsTerminal = true
		}
	}

	if d, ok := wr.(fder); ok {
		if terminal.CanUpdateStatus(d.Fd()) {
			// only use the fancy status code when we're running on a real terminal.
			t.canUpdateStatus = true
			t.fd = d.Fd()
			t.clearCurrentLine = terminal.ClearCurrentLine(t.fd)
			t.moveCursorUp = terminal.MoveCursorUp(t.fd)
		}
		if terminal.OutputIsTerminal(d.Fd()) {
			t.outputIsTerminal = true
		}
	}

	return t
}

// InputIsTerminal returns whether the input is a terminal.
func (t *Terminal) InputIsTerminal() bool {
	return t.inputIsTerminal
}

// InputRaw returns the input reader.
func (t *Terminal) InputRaw() io.ReadCloser {
	return t.rd
}

func (t *Terminal) ReadPassword(ctx context.Context, prompt string) (string, error) {
	if t.InputIsTerminal() {
		t.Flush()
		return terminal.ReadPassword(ctx, int(t.inFd), t.errWriter, prompt)
	}
	if t.OutputIsTerminal() {
		t.Print("reading repository password from stdin")
	}
	return readPassword(t.rd)
}

// readPassword reads the password from the given reader directly.
func readPassword(in io.Reader) (password string, err error) {
	sc := bufio.NewScanner(in)
	sc.Scan()
	if sc.Err() != nil {
		return "", fmt.Errorf("readPassword: %w", sc.Err())
	}
	return sc.Text(), nil
}

// CanUpdateStatus return whether the status output is updated in place.
func (t *Terminal) CanUpdateStatus() bool {
	return t.canUpdateStatus
}

// OutputWriter returns a output writer that is safe for concurrent use with
// other output methods. Output is only shown after a line break.
func (t *Terminal) OutputWriter() io.Writer {
	t.outputWriterOnce.Do(func() {
		t.outputWriter = newLineWriter(t.Print)
	})
	return t.outputWriter
}

// OutputRaw returns the raw output writer. Should only be used if there is no
// other option. Must not be used in combination with Print, Error, SetStatus
// or any other method that writes to the terminal.
func (t *Terminal) OutputRaw() io.Writer {
	t.Flush()
	return t.wr
}

// OutputIsTerminal returns whether the output is a terminal.
func (t *Terminal) OutputIsTerminal() bool {
	return t.outputIsTerminal
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
			if msg.barrier != nil {
				msg.barrier <- struct{}{}
				continue
			}
			if terminal.IsProcessBackground(t.fd) {
				// ignore all messages, do nothing, we are in the background process group
				continue
			}
			if err := t.clearCurrentLine(t.wr, t.fd); err != nil {
				_, _ = fmt.Fprintf(t.errWriter, "write failed: %v\n", err)
				continue
			}

			var dst io.Writer
			if msg.err {
				dst = t.errWriter
			} else {
				dst = t.wr
			}

			if _, err := io.WriteString(dst, msg.line); err != nil {
				_, _ = fmt.Fprintf(t.errWriter, "write failed: %v\n", err)
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
		if err := t.clearCurrentLine(t.wr, t.fd); err != nil {
			_, _ = fmt.Fprintf(t.errWriter, "write failed: %v\n", err)
		}

		_, err := t.wr.Write([]byte(line))
		if err != nil {
			_, _ = fmt.Fprintf(t.errWriter, "write failed: %v\n", err)
		}
	}

	if len(status) > 0 {
		if err := t.moveCursorUp(t.wr, t.fd, len(status)-1); err != nil {
			_, _ = fmt.Fprintf(t.errWriter, "write failed: %v\n", err)
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
			if msg.barrier != nil {
				msg.barrier <- struct{}{}
				continue
			}

			var dst io.Writer
			if msg.err {
				dst = t.errWriter
			} else {
				dst = t.wr
			}

			if _, err := io.WriteString(dst, msg.line); err != nil {
				_, _ = fmt.Fprintf(t.errWriter, "write failed: %v\n", err)
			}

		case stat := <-t.status:
			for _, line := range stat.lines {
				// Ensure that each message ends with exactly one newline.
				if _, err := fmt.Fprintln(t.wr, strings.TrimRight(line, "\n")); err != nil {
					_, _ = fmt.Fprintf(t.errWriter, "write failed: %v\n", err)
				}
			}
		}
	}
}

// Flush waits for all pending messages to be printed.
func (t *Terminal) Flush() {
	ch := make(chan struct{})
	defer close(ch)
	select {
	case t.msg <- message{barrier: ch}:
	case <-t.closed:
	}
	select {
	case <-ch:
	case <-t.closed:
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
