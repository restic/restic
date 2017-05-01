// +build debug

package debug

import (
	"net/http"
	"net/http/httputil"
)

type loggingRoundTripper struct {
	http.RoundTripper
}

// RoundTripper returns a new http.RoundTripper which logs all requests (if
// debug is enabled). When debug is not enabled, upstream is returned.
func RoundTripper(upstream http.RoundTripper) http.RoundTripper {
	return loggingRoundTripper{upstream}
}

func (tr loggingRoundTripper) RoundTrip(req *http.Request) (res *http.Response, err error) {
	trace, err := httputil.DumpRequestOut(req, false)
	if err != nil {
		Log("DumpRequestOut() error: %v\n", err)
	} else {
		Log("------------  HTTP REQUEST -----------\n%s", trace)
	}

	res, err = tr.RoundTripper.RoundTrip(req)
	if err != nil {
		Log("RoundTrip() returned error: %v", err)
	}

	if res != nil {
		trace, err := httputil.DumpResponse(res, false)
		if err != nil {
			Log("DumpResponse() error: %v\n", err)
		} else {
			Log("------------  HTTP RESPONSE ----------\n%s", trace)
		}
	}

	return res, err
}
