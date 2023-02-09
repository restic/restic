package debug

import (
	"net/http"
	"testing"

	"github.com/restic/restic/internal/test"
)

func TestRedactHeader(t *testing.T) {
	secretHeaders := []string{
		"Authorization",
		"X-Auth-Token",
		"X-Auth-Key",
	}

	header := make(http.Header)
	header["Authorization"] = []string{"123"}
	header["X-Auth-Token"] = []string{"1234"}
	header["X-Auth-Key"] = []string{"12345"}
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
	test.Equals(t, "my.host", header["Host"][0])

	delete(header, "X-Auth-Key")
	origHeaders = redactHeader(header)
	_, hasHeader := header["X-Auth-Key"]
	test.Assert(t, !hasHeader, "Unexpected header: %v", header["X-Auth-Key"])

	restoreHeader(header, origHeaders)
	_, hasHeader = header["X-Auth-Key"]
	test.Assert(t, !hasHeader, "Unexpected header: %v", header["X-Auth-Key"])
}
