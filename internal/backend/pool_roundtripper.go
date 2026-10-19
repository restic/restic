package backend

import (
	"io"
	"net/http"
	"sync"
)

// poolRoundTripper spreads requests across several round trippers, each of
// which maintains its own connections.
// When a request is sent, we find a free roundtripper (or create one)
// if there are less than max_num_round_trippers. Otherwise, we use
// the one with the least number of in-flight requests.
//
// For HTTP/2, this avoids multiplexing concurrent requests over a single TCP
// connection, which can severely limit throughput.
type poolRoundTripper struct {
	new_round_tripper      func() http.RoundTripper
	max_num_round_trippers int

	// Protects entries and numInFlight in each poolEntry.
	mutex   sync.Mutex
	entries []*poolEntry
}

type poolEntry struct {
	rt          http.RoundTripper
	numInFlight int
}

func newPoolRoundTripper(new_round_tripper func() http.RoundTripper,
	max_num_round_trippers int) *poolRoundTripper {
	return &poolRoundTripper{
		new_round_tripper:      new_round_tripper,
		max_num_round_trippers: max_num_round_trippers,
	}
}

func (p *poolRoundTripper) acquire() *poolEntry {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	var best *poolEntry
	for _, e := range p.entries {
		if e.numInFlight == 0 {
			best = e
			break
		}
		if best == nil || e.numInFlight < best.numInFlight {
			best = e
		}
	}
	if (best == nil || best.numInFlight > 0) &&
		len(p.entries) < p.max_num_round_trippers {
		best = &poolEntry{rt: p.new_round_tripper()}
		p.entries = append(p.entries, best)
	}
	best.numInFlight++
	return best
}

func (p *poolRoundTripper) release(e *poolEntry) {
	p.mutex.Lock()
	e.numInFlight--
	p.mutex.Unlock()
}

func (p *poolRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	e := p.acquire()
	resp, err := e.rt.RoundTrip(req)
	if err != nil {
		p.release(e)
		return nil, err
	}
	resp.Body = &poolBody{ReadCloser: resp.Body,
		release: sync.OnceFunc(func() { p.release(e) })}
	return resp, nil
}

func (p *poolRoundTripper) CloseIdleConnections() {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	for _, e := range p.entries {
		if c, ok := e.rt.(interface{ CloseIdleConnections() }); ok {
			c.CloseIdleConnections()
		}
	}
}

type poolBody struct {
	io.ReadCloser
	release func()
}

func (b *poolBody) Close() error {
	defer b.release()
	return b.ReadCloser.Close()
}
