package limiter

import (
	"io"

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
	return l.limit(r, l.upstream)
}

func (l staticLimiter) Downstream(r io.Reader) io.Reader {
	return l.limit(r, l.downstream)
}

func (l staticLimiter) limit(r io.Reader, b *ratelimit.Bucket) io.Reader {
	if b == nil {
		return r
	}
	return ratelimit.Reader(r, b)
}

func toByteRate(val int) float64 {
	return float64(val) * 1024.
}
