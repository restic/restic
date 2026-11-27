package debug

import (
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"

	"github.com/restic/restic/internal/errors"
)

type eofDetectRoundTripper struct {
	http.RoundTripper
}

type eofDetectReader struct {
	eofSeen bool
	rd      io.ReadCloser
}

func (rd *eofDetectReader) Read(p []byte) (n int, err error) {
	n, err = rd.rd.Read(p)
	if err == io.EOF {
		rd.eofSeen = true
	}
	return n, err
}

func (rd *eofDetectReader) Close() error {
	if !rd.eofSeen {
		buf, err := io.ReadAll(rd)
		msg := fmt.Sprintf("body not drained, %d bytes not read", len(buf))
		if err != nil {
			msg += fmt.Sprintf(", error: %v", err)
		}

		if len(buf) > 0 {
			if len(buf) > 20 {
				buf = append(buf[:20], []byte("...")...)
			}
			msg += fmt.Sprintf(", body: %q", buf)
		}

		_, _ = fmt.Fprintln(os.Stderr, msg)
		Log("%s: %+v", msg, errors.New("Close()"))
	}
	return rd.rd.Close()
}

func (tr eofDetectRoundTripper) RoundTrip(req *http.Request) (res *http.Response, err error) {
	res, err = tr.RoundTripper.RoundTrip(req)
	if res != nil && res.Body != nil {
		res.Body = &eofDetectReader{rd: res.Body}
	}
	return res, err
}

type loggingRoundTripper struct {
	http.RoundTripper
}

func redactHeader(header http.Header) map[string][]string {
	removedHeaders := make(map[string][]string)
	for _, hdr := range []string{
		"Authorization",
		"X-Auth-Token", // Swift headers
		"X-Auth-Key",
		"X-Amz-Security-Token", // Short-lived S3 tokens
	} {
		origHeader, hasHeader := header[hdr]
		if hasHeader {
			removedHeaders[hdr] = origHeader
			header[hdr] = []string{"**redacted**"}
		}
	}
	return removedHeaders
}

func restoreHeader(header http.Header, origHeaders map[string][]string) {
	maps.Copy(header, origHeaders)
}

func secretQueryParam(key string) bool {
	switch strings.ToLower(key) {
	// "sig" contains the Azure SAS signature
	// "x-amz-signature" and "x-amz-security-token" could show up for
	// presigned s3 URLs although those aren't used at the moment
	case "sig", "x-amz-signature", "x-amz-security-token":
		return true
	default:
		return false
	}
}

// redactRequestURL replaces secret query values. It assumes a well-formed query
// string and as such does not preserve ordering and encoding.
func redactRequestURL(u *url.URL) *url.URL {
	values := u.Query()
	changed := false
	for key, vals := range values {
		if !secretQueryParam(key) {
			continue
		}
		for i := range vals {
			vals[i] = "redacted"
		}
		changed = true
	}
	if !changed {
		return u
	}
	clone := *u
	clone.RawQuery = values.Encode()
	return &clone
}

func (tr loggingRoundTripper) RoundTrip(req *http.Request) (res *http.Response, err error) {
	// save original auth and redact it
	origHeaders := redactHeader(req.Header)
	origURL := req.URL
	req.URL = redactRequestURL(req.URL)

	trace, err := httputil.DumpRequestOut(req, false)
	req.URL = origURL
	if err != nil {
		Log("DumpRequestOut() error: %v\n", err)
	} else {
		Log("------------  HTTP REQUEST -----------\n%s", trace)
	}

	restoreHeader(req.Header, origHeaders)

	res, err = tr.RoundTripper.RoundTrip(req)
	if err != nil {
		Log("RoundTrip() returned error: %v", err)
	}

	if res != nil {
		origHeaders := redactHeader(res.Header)
		trace, err := httputil.DumpResponse(res, false)
		restoreHeader(res.Header, origHeaders)
		if err != nil {
			Log("DumpResponse() error: %v\n", err)
		} else {
			Log("------------  HTTP RESPONSE ----------\n%s", trace)
		}
	}

	return res, err
}
