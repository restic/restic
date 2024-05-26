package backend

import "net/http"

// httpUserAgentRoundTripper is a custom http.RoundTripper that modifies the User-Agent header
// of outgoing HTTP requests.
type httpUserAgentRoundTripper struct {
	userAgent string
	rt        http.RoundTripper
}

func newCustomUserAgentRoundTripper(rt http.RoundTripper, userAgent string) *httpUserAgentRoundTripper {
	return &httpUserAgentRoundTripper{
		rt:        rt,
		userAgent: userAgent,
	}
}

// RoundTrip modifies the User-Agent header of the request and then delegates the request
// to the underlying RoundTripper.
func (c *httpUserAgentRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.Header.Set("User-Agent", c.userAgent)
	return c.rt.RoundTrip(req)
}
