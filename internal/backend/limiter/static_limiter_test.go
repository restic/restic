package limiter

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/restic/restic/internal/test"
	"golang.org/x/time/rate"
)

func TestLimiterWrapping(t *testing.T) {
	reader := bytes.NewReader([]byte{})
	writer := new(bytes.Buffer)

	for _, limits := range []Limits{
		{0, 0},
		{42, 0},
		{0, 42},
		{42, 42},
	} {
		limiter := NewStaticLimiter(limits)

		mustWrapUpstream := limits.UploadKb > 0
		test.Equals(t, limiter.Upstream(reader) != reader, mustWrapUpstream)
		test.Equals(t, limiter.UpstreamWriter(writer) != writer, mustWrapUpstream)

		mustWrapDownstream := limits.DownloadKb > 0
		test.Equals(t, limiter.Downstream(reader) != reader, mustWrapDownstream)
		test.Equals(t, limiter.DownstreamWriter(writer) != writer, mustWrapDownstream)
	}
}

func TestReadLimiter(t *testing.T) {
	reader := bytes.NewReader(make([]byte, 300))
	limiter := rate.NewLimiter(rate.Limit(10000), int(100))
	limReader := rateLimitedReader{reader, limiter}

	n, err := limReader.Read([]byte{})
	test.OK(t, err)
	test.Equals(t, n, 0)

	n, err = limReader.Read(make([]byte, 300))
	test.OK(t, err)
	test.Equals(t, n, 300)

	n, err = limReader.Read([]byte{})
	test.Equals(t, err, io.EOF)
	test.Equals(t, n, 0)
}

func TestWriteLimiter(t *testing.T) {
	writer := &bytes.Buffer{}
	limiter := rate.NewLimiter(rate.Limit(10000), int(100))
	limReader := rateLimitedWriter{writer, limiter}

	n, err := limReader.Write([]byte{})
	test.OK(t, err)
	test.Equals(t, n, 0)

	n, err = limReader.Write(make([]byte, 300))
	test.OK(t, err)
	test.Equals(t, n, 300)
}

type tracedReadCloser struct {
	io.Reader
	Closed bool
}

func newTracedReadCloser(rd io.Reader) *tracedReadCloser {
	return &tracedReadCloser{Reader: rd}
}

func (r *tracedReadCloser) Close() error {
	r.Closed = true
	return nil
}

func TestRoundTripperReader(t *testing.T) {
	limiter := NewStaticLimiter(Limits{42 * 1024, 42 * 1024})
	data := make([]byte, 1234)
	_, err := io.ReadFull(rand.Reader, data)
	test.OK(t, err)

	send := newTracedReadCloser(bytes.NewReader(data))
	var recv *tracedReadCloser

	rt := limiter.Transport(roundTripper(func(req *http.Request) (*http.Response, error) {
		buf := new(bytes.Buffer)
		_, err := io.Copy(buf, req.Body)
		if err != nil {
			return nil, err
		}
		err = req.Body.Close()
		if err != nil {
			return nil, err
		}

		recv = newTracedReadCloser(bytes.NewReader(buf.Bytes()))
		return &http.Response{Body: recv}, nil
	}))

	res, err := rt.RoundTrip(&http.Request{Body: send})
	test.OK(t, err)

	out := new(bytes.Buffer)
	n, err := io.Copy(out, res.Body)
	test.OK(t, err)
	test.Equals(t, int64(len(data)), n)
	test.OK(t, res.Body.Close())

	test.Assert(t, send.Closed, "request body not closed")
	test.Assert(t, recv.Closed, "result body not closed")
	test.Assert(t, bytes.Equal(data, out.Bytes()), "data ping-pong failed")
}

// nolint:bodyclose // the http response is just a mock
func TestRoundTripperCornerCases(t *testing.T) {
	limiter := NewStaticLimiter(Limits{42 * 1024, 42 * 1024})

	rt := limiter.Transport(roundTripper(func(req *http.Request) (*http.Response, error) {
		return &http.Response{}, nil
	}))

	res, err := rt.RoundTrip(&http.Request{})
	test.OK(t, err)
	test.Assert(t, res != nil, "round tripper returned no response")

	rt = limiter.Transport(roundTripper(func(req *http.Request) (*http.Response, error) {
		return nil, fmt.Errorf("error")
	}))

	_, err = rt.RoundTrip(&http.Request{})
	test.Assert(t, err != nil, "round tripper lost an error")
}
