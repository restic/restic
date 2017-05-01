package backend

import (
	"net"
	"net/http"
	"restic/debug"
	"time"
)

// Transport returns a new http.RoundTripper with default settings applied.
func Transport() http.RoundTripper {
	// copied from net/http
	tr := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
			DualStack: true,
		}).DialContext,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	// wrap in the debug round tripper
	return debug.RoundTripper(tr)
}
