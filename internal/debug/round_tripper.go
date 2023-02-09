package debug

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httputil"
	"os"

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
		buf, err := ioutil.ReadAll(rd)
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

		fmt.Fprintln(os.Stderr, msg)
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
	for hdr, val := range origHeaders {
		header[hdr] = val
	}
}

func (tr loggingRoundTripper) RoundTrip(req *http.Request) (res *http.Response, err error) {
	// save original auth and redact it
	origHeaders := redactHeader(req.Header)

	trace, err := httputil.DumpRequestOut(req, false)
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
