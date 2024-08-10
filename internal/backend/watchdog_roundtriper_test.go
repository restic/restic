package backend

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	rtest "github.com/restic/restic/internal/test"
)

func TestRead(t *testing.T) {
	data := []byte("abcdef")
	var ctr int
	kick := func() {
		ctr++
	}
	var closed bool
	onClose := func() {
		closed = true
	}
	isTimeout := func(err error) bool {
		return false
	}

	wd := newWatchdogReadCloser(io.NopCloser(bytes.NewReader(data)), 1, kick, onClose, isTimeout)

	out, err := io.ReadAll(wd)
	rtest.OK(t, err)
	rtest.Equals(t, data, out, "data mismatch")
	// the EOF read also triggers the kick function
	rtest.Equals(t, len(data)*2+2, ctr, "unexpected number of kick calls")

	rtest.Equals(t, false, closed, "close function called too early")
	rtest.OK(t, wd.Close())
	rtest.Equals(t, true, closed, "close function not called")
}

func TestRoundtrip(t *testing.T) {
	t.Parallel()

	// at the higher delay values, it takes longer to transmit the request/response body
	// than the roundTripper timeout
	for _, delay := range []int{0, 1, 10, 20} {
		t.Run(fmt.Sprintf("%v", delay), func(t *testing.T) {
			msg := []byte("ping-pong-data")
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				data, err := io.ReadAll(r.Body)
				if err != nil {
					w.WriteHeader(500)
					return
				}
				w.WriteHeader(200)

				// slowly send the reply
				for len(data) >= 2 {
					_, _ = w.Write(data[:2])
					w.(http.Flusher).Flush()
					data = data[2:]
					time.Sleep(time.Duration(delay) * time.Millisecond)
				}
				_, _ = w.Write(data)
			}))
			defer srv.Close()

			rt := newWatchdogRoundtripper(http.DefaultTransport, 100*time.Millisecond, 2)
			req, err := http.NewRequestWithContext(context.TODO(), "GET", srv.URL, io.NopCloser(newSlowReader(bytes.NewReader(msg), time.Duration(delay)*time.Millisecond)))
			rtest.OK(t, err)

			resp, err := rt.RoundTrip(req)
			rtest.OK(t, err)
			rtest.Equals(t, 200, resp.StatusCode, "unexpected status code")

			response, err := io.ReadAll(resp.Body)
			rtest.OK(t, err)
			rtest.Equals(t, msg, response, "unexpected response")

			rtest.OK(t, resp.Body.Close())
		})
	}
}

func TestCanceledRoundtrip(t *testing.T) {
	rt := newWatchdogRoundtripper(http.DefaultTransport, time.Second, 2)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", "http://some.random.url.dfdgsfg", nil)
	rtest.OK(t, err)

	resp, err := rt.RoundTrip(req)
	rtest.Equals(t, context.Canceled, err)
	// make linter happy
	if resp != nil {
		rtest.OK(t, resp.Body.Close())
	}
}

type slowReader struct {
	data  io.Reader
	delay time.Duration
}

func newSlowReader(data io.Reader, delay time.Duration) *slowReader {
	return &slowReader{
		data:  data,
		delay: delay,
	}
}

func (s *slowReader) Read(p []byte) (n int, err error) {
	time.Sleep(s.delay)
	return s.data.Read(p)
}

func TestUploadTimeout(t *testing.T) {
	t.Parallel()

	msg := []byte("ping")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(500)
			return
		}
		t.Error("upload should have been canceled")
	}))
	defer srv.Close()

	rt := newWatchdogRoundtripper(http.DefaultTransport, 10*time.Millisecond, 1024)
	req, err := http.NewRequestWithContext(context.TODO(), "GET", srv.URL, io.NopCloser(newSlowReader(bytes.NewReader(msg), 100*time.Millisecond)))
	rtest.OK(t, err)

	resp, err := rt.RoundTrip(req)
	rtest.Equals(t, context.Canceled, err)
	// make linter happy
	if resp != nil {
		rtest.OK(t, resp.Body.Close())
	}
}

func TestProcessingTimeout(t *testing.T) {
	t.Parallel()

	msg := []byte("ping")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(500)
			return
		}
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	rt := newWatchdogRoundtripper(http.DefaultTransport, 10*time.Millisecond, 1024)
	req, err := http.NewRequestWithContext(context.TODO(), "GET", srv.URL, io.NopCloser(bytes.NewReader(msg)))
	rtest.OK(t, err)

	resp, err := rt.RoundTrip(req)
	rtest.Equals(t, context.Canceled, err)
	// make linter happy
	if resp != nil {
		rtest.OK(t, resp.Body.Close())
	}
}

func TestDownloadTimeout(t *testing.T) {
	t.Parallel()

	msg := []byte("ping")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(500)
			return
		}
		w.WriteHeader(200)
		_, _ = w.Write(data[:2])
		w.(http.Flusher).Flush()
		data = data[2:]

		time.Sleep(100 * time.Millisecond)
		_, _ = w.Write(data)

	}))
	defer srv.Close()

	rt := newWatchdogRoundtripper(http.DefaultTransport, 25*time.Millisecond, 1024)
	req, err := http.NewRequestWithContext(context.TODO(), "GET", srv.URL, io.NopCloser(bytes.NewReader(msg)))
	rtest.OK(t, err)

	resp, err := rt.RoundTrip(req)
	rtest.OK(t, err)
	rtest.Equals(t, 200, resp.StatusCode, "unexpected status code")

	_, err = io.ReadAll(resp.Body)
	rtest.Equals(t, errRequestTimeout, err, "response download not canceled")
	rtest.OK(t, resp.Body.Close())
}
