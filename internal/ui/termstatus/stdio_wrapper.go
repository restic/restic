package termstatus

import (
	"bytes"
	"io"
)

// WrapStdio returns line-buffering replacements for os.Stdout and os.Stderr.
// On Close, the remaining bytes are written, followed by a line break.
func WrapStdio(term *Terminal) (stdout, stderr io.WriteCloser) {
	return newLineWriter(term.Print), newLineWriter(term.Error)
}

type lineWriter struct {
	buf   bytes.Buffer
	print func(string)
}

var _ io.WriteCloser = &lineWriter{}

func newLineWriter(print func(string)) *lineWriter {
	return &lineWriter{print: print}
}

func (w *lineWriter) Write(data []byte) (n int, err error) {
	n, err = w.buf.Write(data)
	if err != nil {
		return n, err
	}

	// look for line breaks
	buf := w.buf.Bytes()
	i := bytes.LastIndexByte(buf, '\n')
	if i != -1 {
		w.print(string(buf[:i+1]))
		w.buf.Next(i + 1)
	}

	return n, err
}

func (w *lineWriter) Close() error {
	if w.buf.Len() > 0 {
		w.print(string(append(w.buf.Bytes(), '\n')))
	}
	return nil
}
