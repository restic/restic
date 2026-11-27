package debug

import (
	"bytes"
	"io"
	"log"
	"net/http"
	"strings"
	"testing"

	"github.com/restic/restic/internal/test"
)

func TestRedactHeader(t *testing.T) {
	secretHeaders := []string{
		"Authorization",
		"X-Auth-Token",
		"X-Auth-Key",
		"X-Amz-Security-Token",
	}

	header := make(http.Header)
	header["Authorization"] = []string{"123"}
	header["X-Auth-Token"] = []string{"1234"}
	header["X-Auth-Key"] = []string{"12345"}
	header["X-Amz-Security-Token"] = []string{"session"}
	header["Host"] = []string{"my.host"}

	origHeaders := redactHeader(header)

	for _, hdr := range secretHeaders {
		test.Equals(t, "**redacted**", header[hdr][0])
	}
	test.Equals(t, "my.host", header["Host"][0])

	restoreHeader(header, origHeaders)
	test.Equals(t, "123", header["Authorization"][0])
	test.Equals(t, "1234", header["X-Auth-Token"][0])
	test.Equals(t, "12345", header["X-Auth-Key"][0])
	test.Equals(t, "session", header["X-Amz-Security-Token"][0])
	test.Equals(t, "my.host", header["Host"][0])

	delete(header, "X-Auth-Key")
	origHeaders = redactHeader(header)
	_, hasHeader := header["X-Auth-Key"]
	test.Assert(t, !hasHeader, "Unexpected header: %v", header["X-Auth-Key"])

	restoreHeader(header, origHeaders)
	_, hasHeader = header["X-Auth-Key"]
	test.Assert(t, !hasHeader, "Unexpected header: %v", header["X-Auth-Key"])
}

func TestLoggingRoundTripperRedactsSecrets(t *testing.T) {
	var buf bytes.Buffer
	oldLogger, oldEnabled := opts.logger, opts.isEnabled
	opts.logger = log.New(&buf, "", 0)
	opts.isEnabled = true
	t.Cleanup(func() {
		opts.logger = oldLogger
		opts.isEnabled = oldEnabled
	})

	req, err := http.NewRequest(http.MethodGet, "https://account.blob.core.windows.net/container?comp=list&sig=sas-secret&X-Amz-Signature=aws-secret&X-Amz-Security-Token=query-token", nil)
	test.OK(t, err)
	req.Header.Set("Authorization", "basic-secret")
	req.Header.Set("X-Amz-Security-Token", "header-token")

	var sent *http.Request
	tr := loggingRoundTripper{RoundTripper: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		sent = r.Clone(r.Context())
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Proto:      "HTTP/1.1",
			ProtoMajor: 1,
			ProtoMinor: 1,
			Header:     http.Header{"X-Amz-Security-Token": []string{"response-token"}},
			Body:       io.NopCloser(strings.NewReader("")),
			Request:    r,
		}, nil
	})}

	res, err := tr.RoundTrip(req)
	test.OK(t, err)
	defer func() {
		test.OK(t, res.Body.Close())
	}()
	test.Equals(t, "response-token", res.Header.Get("X-Amz-Security-Token"))
	test.Equals(t, "basic-secret", sent.Header.Get("Authorization"))
	test.Equals(t, "header-token", sent.Header.Get("X-Amz-Security-Token"))
	test.Equals(t, "sas-secret", sent.URL.Query().Get("sig"))
	test.Equals(t, "aws-secret", sent.URL.Query().Get("X-Amz-Signature"))
	test.Equals(t, "query-token", sent.URL.Query().Get("X-Amz-Security-Token"))
	test.Equals(t, "list", sent.URL.Query().Get("comp"))

	logText := buf.String()
	for _, secret := range []string{"sas-secret", "aws-secret", "query-token", "header-token", "basic-secret", "response-token"} {
		test.Assert(t, !strings.Contains(logText, secret), "secret %q leaked into debug log: %s", secret, logText)
	}
	test.Assert(t, strings.Contains(logText, "sig=redacted"), "missing redacted sas: %s", logText)
	test.Assert(t, strings.Contains(logText, "X-Amz-Signature=redacted"), "missing redacted signature: %s", logText)
	test.Assert(t, strings.Contains(logText, "X-Amz-Security-Token=redacted"), "missing redacted query token: %s", logText)
	test.Assert(t, strings.Contains(logText, "comp=list"), "ordinary query parameter was dropped: %s", logText)
	test.Assert(t, strings.Contains(logText, "X-Amz-Security-Token: **redacted**"), "missing redacted header: %s", logText)
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}
