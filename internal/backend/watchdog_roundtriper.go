package backend

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync/atomic"
	"time"
)

var errRequestTimeout = fmt.Errorf("request timeout")

// watchdogRoundtripper cancels an http request if an upload or download did not make progress
// within timeout. The time between fully sending the request and receiving an response is also
// limited by this timeout. This ensures that stuck requests are cancelled after some time.
//
// The roundtriper makes the assumption that the upload and download happen continuously. In particular,
// the caller must not make long pauses between individual read requests from the response body.
type watchdogRoundtripper struct {
	rt        http.RoundTripper
	timeout   time.Duration
	chunkSize int
}

var _ http.RoundTripper = &watchdogRoundtripper{}

func newWatchdogRoundtripper(rt http.RoundTripper, timeout time.Duration, chunkSize int) *watchdogRoundtripper {
	return &watchdogRoundtripper{
		rt:        rt,
		timeout:   timeout,
		chunkSize: chunkSize,
	}
}

func (w *watchdogRoundtripper) RoundTrip(req *http.Request) (*http.Response, error) {
	timer := time.NewTimer(w.timeout)
	ctx, cancel := context.WithCancel(req.Context())
	timedOut := &atomic.Bool{}

	// cancel context if timer expires
	go func() {
		defer timer.Stop()
		select {
		case <-timer.C:
			timedOut.Store(true)
			cancel()
		case <-ctx.Done():
		}
	}()

	kick := func() {
		timer.Reset(w.timeout)
	}
	isTimeout := func(err error) bool {
		return timedOut.Load() && errors.Is(err, context.Canceled)
	}

	req = req.Clone(ctx)
	if req.Body != nil {
		// kick watchdog timer as long as uploading makes progress
		req.Body = newWatchdogReadCloser(req.Body, w.chunkSize, kick, nil, isTimeout)
	}

	resp, err := w.rt.RoundTrip(req)
	if err != nil {
		return nil, err
	}

	// kick watchdog timer as long as downloading makes progress
	// cancel context to stop goroutine once response body is closed
	resp.Body = newWatchdogReadCloser(resp.Body, w.chunkSize, kick, cancel, isTimeout)
	return resp, nil
}

func newWatchdogReadCloser(rc io.ReadCloser, chunkSize int, kick func(), close func(), isTimeout func(err error) bool) *watchdogReadCloser {
	return &watchdogReadCloser{
		rc:        rc,
		chunkSize: chunkSize,
		kick:      kick,
		close:     close,
		isTimeout: isTimeout,
	}
}

type watchdogReadCloser struct {
	rc        io.ReadCloser
	chunkSize int
	kick      func()
	close     func()
	isTimeout func(err error) bool
}

var _ io.ReadCloser = &watchdogReadCloser{}

func (w *watchdogReadCloser) Read(p []byte) (n int, err error) {
	w.kick()

	// Read is not required to fill the whole passed in byte slice
	// Thus, keep things simple and just stay within our chunkSize.
	if len(p) > w.chunkSize {
		p = p[:w.chunkSize]
	}
	n, err = w.rc.Read(p)
	w.kick()

	if err != nil && w.isTimeout(err) {
		err = errRequestTimeout
	}
	return n, err
}

func (w *watchdogReadCloser) Close() error {
	if w.close != nil {
		w.close()
	}
	return w.rc.Close()
}
