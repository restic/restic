package termstatus

import (
	"bytes"
	"io"
	"sync"
)

type lineWriter struct {
	m     sync.Mutex
	buf   bytes.Buffer
	print func(string)
}

var _ io.WriteCloser = &lineWriter{}

func newLineWriter(print func(string)) *lineWriter {
	return &lineWriter{print: print}
}

func (w *lineWriter) Write(data []byte) (n int, err error) {
	w.m.Lock()
	defer w.m.Unlock()

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
	w.m.Lock()
	defer w.m.Unlock()

	if w.buf.Len() > 0 {
		w.print(string(append(w.buf.Bytes(), '\n')))
	}
	return nil
}
