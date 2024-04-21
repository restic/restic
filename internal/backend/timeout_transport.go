package backend

import (
	"net"
	"os"
	"sync"
	"time"
)

// timeoutConn will timeout if no read or write progress is made for progressTimeout.
// This ensures that stuck network connections are interrupted after some time.
// By using a timeoutConn within a http transport (via DialContext), sending / receing
// the request / response body is guarded with a timeout. The read progress part also
// limits the time until a response header must be received.
//
// The progressTimeout must be larger than the IdleConnTimeout of the http transport.
//
// The http2.Transport offers a similar functionality via WriteByteTimeout & ReadIdleTimeout.
// However, those are not available for HTTP/1 connections. Thus, there's no builtin way to
// enforce progress for sending the request body or reading the response body.
// See https://github.com/restic/restic/issues/4193#issuecomment-2067988727 for details.
type timeoutConn struct {
	conn net.Conn
	// timeout within which a read/write must make progress, otherwise a connection is considered broken
	// if no read/write is pending, then the timeout is inactive
	progressTimeout time.Duration

	// all access to fields below must hold m
	m sync.Mutex

	// user defined read/write deadline
	readDeadline  time.Time
	writeDeadline time.Time
	// timestamp of last successful write (at least one byte)
	lastWrite time.Time
}

var _ net.Conn = &timeoutConn{}

func newTimeoutConn(conn net.Conn, progressTimeout time.Duration) (*timeoutConn, error) {
	// reset timeouts to ensure a consistent state
	err := conn.SetDeadline(time.Time{})
	if err != nil {
		return nil, err
	}

	return &timeoutConn{
		conn:            conn,
		progressTimeout: progressTimeout,
	}, nil
}

func (t *timeoutConn) Write(p []byte) (n int, err error) {
	t.m.Lock()
	timeout := t.writeDeadline
	t.m.Unlock()
	var zero time.Time
	if timeout != zero {
		// fall back to standard behavior if a timeout was set explicitly
		n, err := t.conn.Write(p)
		if n > 0 {
			t.m.Lock()
			t.lastWrite = time.Now()
			t.m.Unlock()
		}
		return n, err
	}

	// based on http2stickyErrWriter.Write from go/src/net/http/h2_bundle.go
	for {
		_ = t.conn.SetWriteDeadline(time.Now().Add(t.progressTimeout))

		nn, err := t.conn.Write(p[n:])
		n += nn
		if nn > 0 {
			// track write progress
			t.m.Lock()
			t.lastWrite = time.Now()
			t.m.Unlock()
		}

		if n < len(p) && nn > 0 && err == os.ErrDeadlineExceeded {
			// some data is still left to send, keep going as long as there is some progress
			continue
		}

		t.m.Lock()
		// restore configured deadline
		_ = t.conn.SetWriteDeadline(t.writeDeadline)
		t.m.Unlock()
		return n, err
	}
}

func (t *timeoutConn) Read(b []byte) (n int, err error) {
	t.m.Lock()
	timeout := t.readDeadline
	t.m.Unlock()
	var zero time.Time
	if timeout != zero {
		// fall back to standard behavior if a timeout was set explicitly
		return t.conn.Read(b)
	}

	var start = time.Now()

	for {
		_ = t.conn.SetReadDeadline(start.Add(t.progressTimeout))

		nn, err := t.conn.Read(b)
		t.m.Lock()
		lastWrite := t.lastWrite
		t.m.Unlock()
		if nn == 0 && err == os.ErrDeadlineExceeded && lastWrite.After(start) {
			// deadline exceeded, but write made some progress in the meantime
			start = lastWrite
			continue
		}

		t.m.Lock()
		// restore configured deadline
		_ = t.conn.SetReadDeadline(t.readDeadline)
		t.m.Unlock()
		return nn, err
	}
}

func (t *timeoutConn) Close() error {
	return t.conn.Close()
}

func (t *timeoutConn) LocalAddr() net.Addr {
	return t.conn.LocalAddr()
}

func (t *timeoutConn) RemoteAddr() net.Addr {
	return t.conn.RemoteAddr()
}

func (t *timeoutConn) SetDeadline(d time.Time) error {
	err := t.SetReadDeadline(d)
	err2 := t.SetWriteDeadline(d)
	if err != nil {
		return err
	}
	return err2
}

func (t *timeoutConn) SetReadDeadline(d time.Time) error {
	t.m.Lock()
	defer t.m.Unlock()

	// track timeout modifications, as the current timeout cannot be queried
	err := t.conn.SetReadDeadline(d)
	if err != nil {
		return err
	}
	t.readDeadline = d
	return nil
}

func (t *timeoutConn) SetWriteDeadline(d time.Time) error {
	t.m.Lock()
	defer t.m.Unlock()

	err := t.conn.SetWriteDeadline(d)
	if err != nil {
		return err
	}
	t.writeDeadline = d
	return nil
}
