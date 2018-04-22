package ui

import (
	"bytes"
	"io"

	"github.com/restic/restic/internal/ui/termstatus"
)

// StdioWrapper provides stdout and stderr integration with termstatus.
type StdioWrapper struct {
	stdout *lineWriter
	stderr *lineWriter
}

// NewStdioWrapper initializes a new stdio wrapper that can be used in place of
// os.Stdout or os.Stderr.
func NewStdioWrapper(term *termstatus.Terminal) *StdioWrapper {
	return &StdioWrapper{
		stdout: newLineWriter(term.Print),
		stderr: newLineWriter(term.Error),
	}
}

// Stdout returns a writer that is line buffered and can be used in place of
// os.Stdout. On Close(), the remaining bytes are written, followed by a line
// break.
func (w *StdioWrapper) Stdout() io.WriteCloser {
	return w.stdout
}

// Stderr returns a writer that is line buffered and can be used in place of
// os.Stderr. On Close(), the remaining bytes are written, followed by a line
// break.
func (w *StdioWrapper) Stderr() io.WriteCloser {
	return w.stderr
}

type lineWriter struct {
	buf   *bytes.Buffer
	print func(string)
}

var _ io.WriteCloser = &lineWriter{}

func newLineWriter(print func(string)) *lineWriter {
	return &lineWriter{buf: bytes.NewBuffer(nil), print: print}
}

func (w *lineWriter) Write(data []byte) (n int, err error) {
	n, err = w.buf.Write(data)
	if err != nil {
		return n, err
	}

	// look for line breaks
	buf := w.buf.Bytes()
	skip := 0
	for i := 0; i < len(buf); {
		if buf[i] == '\n' {
			// found line
			w.print(string(buf[:i+1]))
			buf = buf[i+1:]
			skip += i + 1
			i = 0
			continue
		}

		i++
	}

	_ = w.buf.Next(skip)

	return n, err
}

func (w *lineWriter) Flush() error {
	if w.buf.Len() > 0 {
		w.print(string(append(w.buf.Bytes(), '\n')))
	}
	return nil
}

func (w *lineWriter) Close() error {
	return w.Flush()
}
