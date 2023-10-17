package limiter

import (
	"context"
	"io"
	"net/http"

	"golang.org/x/time/rate"
)

type staticLimiter struct {
	upstream   *rate.Limiter
	downstream *rate.Limiter
}

// Limits represents static upload and download limits.
// For both, zero means unlimited.
type Limits struct {
	UploadKb   int
	DownloadKb int
}

// NewStaticLimiter constructs a Limiter with a fixed (static) upload and
// download rate cap
func NewStaticLimiter(l Limits) Limiter {
	var (
		upstreamBucket   *rate.Limiter
		downstreamBucket *rate.Limiter
	)

	if l.UploadKb > 0 {
		upstreamBucket = rate.NewLimiter(rate.Limit(toByteRate(l.UploadKb)), int(toByteRate(l.UploadKb)))
	}

	if l.DownloadKb > 0 {
		downstreamBucket = rate.NewLimiter(rate.Limit(toByteRate(l.DownloadKb)), int(toByteRate(l.DownloadKb)))
	}

	return staticLimiter{
		upstream:   upstreamBucket,
		downstream: downstreamBucket,
	}
}

func (l staticLimiter) Upstream(r io.Reader) io.Reader {
	return l.limitReader(r, l.upstream)
}

func (l staticLimiter) UpstreamWriter(w io.Writer) io.Writer {
	return l.limitWriter(w, l.upstream)
}

func (l staticLimiter) Downstream(r io.Reader) io.Reader {
	return l.limitReader(r, l.downstream)
}

func (l staticLimiter) DownstreamWriter(w io.Writer) io.Writer {
	return l.limitWriter(w, l.downstream)
}

type roundTripper func(*http.Request) (*http.Response, error)

func (rt roundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return rt(req)
}

func (l staticLimiter) roundTripper(rt http.RoundTripper, req *http.Request) (*http.Response, error) {
	type readCloser struct {
		io.Reader
		io.Closer
	}

	if req.Body != nil {
		req.Body = &readCloser{
			Reader: l.Upstream(req.Body),
			Closer: req.Body,
		}
	}

	res, err := rt.RoundTrip(req)

	if res != nil && res.Body != nil {
		res.Body = &readCloser{
			Reader: l.Downstream(res.Body),
			Closer: res.Body,
		}
	}

	return res, err
}

// Transport returns an HTTP transport limited with the limiter l.
func (l staticLimiter) Transport(rt http.RoundTripper) http.RoundTripper {
	return roundTripper(func(req *http.Request) (*http.Response, error) {
		return l.roundTripper(rt, req)
	})
}

func (l staticLimiter) limitReader(r io.Reader, b *rate.Limiter) io.Reader {
	if b == nil {
		return r
	}
	return &rateLimitedReader{r, b}
}

type rateLimitedReader struct {
	reader io.Reader
	bucket *rate.Limiter
}

func (r *rateLimitedReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	if err := consumeTokens(n, r.bucket); err != nil {
		return n, err
	}
	return n, err
}

func (l staticLimiter) limitWriter(w io.Writer, b *rate.Limiter) io.Writer {
	if b == nil {
		return w
	}
	return &rateLimitedWriter{w, b}
}

type rateLimitedWriter struct {
	writer io.Writer
	bucket *rate.Limiter
}

func (w *rateLimitedWriter) Write(buf []byte) (int, error) {
	if err := consumeTokens(len(buf), w.bucket); err != nil {
		return 0, err
	}
	return w.writer.Write(buf)
}

func consumeTokens(tokens int, bucket *rate.Limiter) error {
	// bucket allows waiting for at most Burst() tokens at once
	maxWait := bucket.Burst()
	for tokens > maxWait {
		if err := bucket.WaitN(context.Background(), maxWait); err != nil {
			return err
		}
		tokens -= maxWait
	}
	return bucket.WaitN(context.Background(), tokens)
}

func toByteRate(val int) float64 {
	return float64(val) * 1024.
}
