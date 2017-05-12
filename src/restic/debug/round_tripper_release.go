// +build !debug

package debug

import "net/http"

// RoundTripper returns a new http.RoundTripper which logs all requests (if
// debug is enabled). When debug is not enabled, upstream is returned.
func RoundTripper(upstream http.RoundTripper) http.RoundTripper {
	return upstream
}
