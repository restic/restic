package backend

import (
	"errors"
	"net/http"
	"testing"
)

type fakeRT struct{ err error }

func (f fakeRT) RoundTrip(*http.Request) (*http.Response, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &http.Response{Body: http.NoBody}, nil
}

func newRequest(t *testing.T, url string) *http.Request {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	return req
}

func numInFlight(p *poolRoundTripper) (n int) {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	for _, e := range p.entries {
		n += e.numInFlight
	}
	return n
}

// Sequential requests should reuse one round tripper.
func TestPoolRoundTripperSequentialReuse(t *testing.T) {
	p := newPoolRoundTripper(func() http.RoundTripper { return fakeRT{} }, 3)
	req := newRequest(t, "http://example.com")

	for range 3 {
		resp, _ := p.RoundTrip(req)
		_ = resp.Body.Close()
	}
	if len(p.entries) != 1 {
		t.Fatalf("sequential requests: expected 1 round tripper, got %d", len(p.entries))
	}
}

// Requests with open bodies get separate round trippers, up to the limit.
func TestPoolRoundTripperMultipleRoundTrippers(t *testing.T) {
	p := newPoolRoundTripper(func() http.RoundTripper { return fakeRT{} }, 3)
	req := newRequest(t, "http://example.com")
	var resps []*http.Response
	for range 5 {
		resp, _ := p.RoundTrip(req)
		resps = append(resps, resp)
	}
	if len(p.entries) != 3 || numInFlight(p) != 5 {
		t.Fatalf("expected 3 round trippers with 5 requests in flight, got %d with %d", len(p.entries), numInFlight(p))
	}

	// Closing a body (even twice) should release its round tripper.
	for _, resp := range resps {
		_ = resp.Body.Close()
		_ = resp.Body.Close()
	}
	if numInFlight(p) != 0 {
		t.Fatalf("expected no requests in flight, got %d", numInFlight(p))
	}
}

func TestPoolRoundTripperError(t *testing.T) {
	p := newPoolRoundTripper(func() http.RoundTripper { return fakeRT{err: errors.New("failed")} }, 3)
	req := newRequest(t, "http://example.com")
	for range 3 {
		if _, err := p.RoundTrip(req); err == nil {
			t.Fatal("expected error")
		}
	}
	if len(p.entries) != 1 || numInFlight(p) != 0 {
		t.Fatal("expected failed requests to release their round tripper")
	}
}
