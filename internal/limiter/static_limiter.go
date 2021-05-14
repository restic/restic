package limiter

import (
	"io"
	"net/http"

	"github.com/juju/ratelimit"
)

type staticLimiter struct {
	upstream   *ratelimit.Bucket
	downstream *ratelimit.Bucket
}

// NewStaticLimiter constructs a Limiter with a fixed (static) upload and
// download rate cap
func NewStaticLimiter(uploadKb, downloadKb int) Limiter {
	var (
		upstreamBucket   *ratelimit.Bucket
		downstreamBucket *ratelimit.Bucket
	)

	if uploadKb > 0 {
		upstreamBucket = ratelimit.NewBucketWithRate(toByteRate(uploadKb), int64(toByteRate(uploadKb)))
	}

	if downloadKb > 0 {
		downstreamBucket = ratelimit.NewBucketWithRate(toByteRate(downloadKb), int64(toByteRate(downloadKb)))
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

func (l staticLimiter) limitReader(r io.Reader, b *ratelimit.Bucket) io.Reader {
	if b == nil {
		return r
	}
	return ratelimit.Reader(r, b)
}

func (l staticLimiter) limitWriter(w io.Writer, b *ratelimit.Bucket) io.Writer {
	if b == nil {
		return w
	}
	return ratelimit.Writer(w, b)
}

func toByteRate(val int) float64 {
	return float64(val) * 1024.
}
