// +build debug

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
