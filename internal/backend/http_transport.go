package backend

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"time"

	"github.com/restic/restic/internal/debug"
)

// Transport returns a new http.RoundTripper with default settings applied. If
// a custom rootCertFilename is non-empty, it must point to a valid PEM file,
// otherwise the function will return an error.
func Transport(rootCertFilenames []string) (http.RoundTripper, error) {
	// copied from net/http
	tr := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
			DualStack: true,
		}).DialContext,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	if rootCertFilenames == nil {
		return debug.RoundTripper(tr), nil
	}

	p := x509.NewCertPool()
	for _, filename := range rootCertFilenames {
		if filename == "" {
			return nil, fmt.Errorf("empty filename for root certificate supplied")
		}
		b, err := ioutil.ReadFile(filename)
		if err != nil {
			return nil, fmt.Errorf("unable to read root certificate: %v", err)
		}
		if ok := p.AppendCertsFromPEM(b); !ok {
			return nil, fmt.Errorf("cannot parse root certificate from %q", filename)
		}
	}

	tr.TLSClientConfig = &tls.Config{
		RootCAs: p,
	}

	// wrap in the debug round tripper
	return debug.RoundTripper(tr), nil
}
