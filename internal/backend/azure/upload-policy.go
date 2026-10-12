package azure

import (
	"io"
	"net/http"
	"sync"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
)

type uploadPolicy struct{}

func (uploadPolicy) Do(req *policy.Request) (*http.Response, error) {
	if req.Raw().Body == nil {
		return req.Next()
	}

	// The SDK can rewind the input while net/http is still reading the previous
	// attempt. Retire that attempt's body before the SDK or caller can reuse it.
	req = req.Clone(req.Raw().Context())
	raw := req.Raw()
	body := &uploadBody{ReadCloser: raw.Body}
	raw.Body = body
	var mu sync.Mutex
	retired := false
	if getBody := raw.GetBody; getBody != nil {
		raw.GetBody = func() (io.ReadCloser, error) {
			mu.Lock()
			defer mu.Unlock()
			if retired {
				return nil, io.ErrClosedPipe
			}
			// Native HTTP retries also rewind the SDK input through GetBody.
			// Retire their previous body before allowing that rewind.
			_ = body.Close()
			rc, err := getBody()
			if err != nil {
				return nil, err
			}
			body = &uploadBody{ReadCloser: rc}
			return body, nil
		}
	}
	// The SDK's downstream body-download policy must consume and cache the
	// response first. Retiring the writer earlier could truncate an HTTP/1
	// error response when the transport closes the connection.
	resp, err := req.Next()
	// Match net/http: request body close errors do not replace the response or
	// transport error. Closing only the SDK's per-attempt body preserves the
	// caller's input ownership and leaves it available for the next retry.
	mu.Lock()
	retired = true
	_ = body.Close()
	mu.Unlock()
	return resp, err
}

type uploadBody struct {
	io.ReadCloser
	mu     sync.Mutex
	closed bool
}

func (b *uploadBody) Read(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return 0, io.ErrClosedPipe
	}
	return b.ReadCloser.Read(p)
}

func (b *uploadBody) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil
	}
	b.closed = true
	return b.ReadCloser.Close()
}
