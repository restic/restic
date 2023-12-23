package limiter

import (
	"io"
	"net/http"
)

// Limiter defines an interface that implementers can use to rate limit I/O
// according to some policy defined and configured by the implementer.
type Limiter interface {
	// Upstream returns a rate limited reader that is intended to be used in
	// uploads.
	Upstream(r io.Reader) io.Reader

	// UpstreamWriter returns a rate limited writer that is intended to be used
	// in uploads.
	UpstreamWriter(w io.Writer) io.Writer

	// Downstream returns a rate limited reader that is intended to be used
	// for downloads.
	Downstream(r io.Reader) io.Reader

	// Downstream returns a rate limited reader that is intended to be used
	// for downloads.
	DownstreamWriter(r io.Writer) io.Writer

	// Transport returns an http.RoundTripper limited with the limiter.
	Transport(http.RoundTripper) http.RoundTripper
}
