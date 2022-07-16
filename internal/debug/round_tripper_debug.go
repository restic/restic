//go:build debug
// +build debug

package debug

import "net/http"

// RoundTripper returns a new http.RoundTripper which logs all requests (if
// debug is enabled). When debug is not enabled, upstream is returned.
func RoundTripper(upstream http.RoundTripper) http.RoundTripper {
	eofRoundTripper := eofDetectRoundTripper{upstream}
	if opts.isEnabled {
		// only use loggingRoundTripper if the debug log is configured
		return loggingRoundTripper{eofRoundTripper}
	}
	return eofRoundTripper
}
