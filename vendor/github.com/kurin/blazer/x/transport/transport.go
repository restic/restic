// Copyright 2017, Google
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package transport provides http.RoundTrippers that may be useful to clients
// of Blazer.
package transport

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"net/http"
	"strings"
	"sync/atomic"
	"time"
)

// WithFailures returns an http.RoundTripper that wraps an existing
// RoundTripper, causing failures according to the options given.  If rt is
// nil, the http.DefaultTransport is wrapped.
func WithFailures(rt http.RoundTripper, opts ...FailureOption) http.RoundTripper {
	if rt == nil {
		rt = http.DefaultTransport
	}
	o := &options{
		rt: rt,
	}
	for _, opt := range opts {
		opt(o)
	}
	return o
}

type options struct {
	pathSubstrings []string
	failureRate    float64
	status         int
	stall          time.Duration
	rt             http.RoundTripper
	msg            string
	trg            *triggerReaderGroup
}

func (o *options) doRequest(req *http.Request) (*http.Response, error) {
	if o.trg != nil && req.Body != nil {
		req.Body = o.trg.new(req.Body)
	}
	resp, err := o.rt.RoundTrip(req)
	if resp != nil && o.trg != nil {
		resp.Body = o.trg.new(resp.Body)
	}
	return resp, err
}

func (o *options) RoundTrip(req *http.Request) (*http.Response, error) {
	// TODO: fix triggering conditions
	if rand.Float64() > o.failureRate {
		return o.doRequest(req)
	}

	var match bool
	if len(o.pathSubstrings) == 0 {
		match = true
	}
	for _, ss := range o.pathSubstrings {
		if strings.Contains(req.URL.Path, ss) {
			match = true
			break
		}
	}
	if !match {
		return o.doRequest(req)
	}

	if o.status > 0 {
		resp := &http.Response{
			Status:     fmt.Sprintf("%d %s", o.status, http.StatusText(o.status)),
			StatusCode: o.status,
			Body:       ioutil.NopCloser(strings.NewReader(o.msg)),
			Request:    req,
		}
		return resp, nil
	}

	if o.stall > 0 {
		ctx := req.Context()
		select {
		case <-time.After(o.stall):
		case <-ctx.Done():
		}
	}
	return o.doRequest(req)
}

// A FailureOption specifies the kind of failure that the RoundTripper should
// display.
type FailureOption func(*options)

// MatchPathSubstring restricts the RoundTripper to URLs whose paths contain
// the given string.  The default behavior is to match all paths.
func MatchPathSubstring(s string) FailureOption {
	return func(o *options) {
		o.pathSubstrings = append(o.pathSubstrings, s)
	}
}

// FailureRate causes the RoundTripper to fail a certain percentage of the
// time.  rate should be a number between 0 and 1, where 0 will never fail and
// 1 will always fail.  The default is never to fail.
func FailureRate(rate float64) FailureOption {
	return func(o *options) {
		o.failureRate = rate
	}
}

// Response simulates a given status code.  The returned http.Response will
// have its Status, StatusCode, and Body (with any predefined message) set.
func Response(status int) FailureOption {
	return func(o *options) {
		o.status = status
	}
}

// Stall simulates a network connection failure by stalling for the given
// duration.
func Stall(dur time.Duration) FailureOption {
	return func(o *options) {
		o.stall = dur
	}
}

// If a specific Response is requested, the body will have the given message
// set.
func Body(msg string) FailureOption {
	return func(o *options) {
		o.msg = msg
	}
}

// Trigger will raise the RoundTripper's failure rate to 100% when the given
// context is closed.
func Trigger(ctx context.Context) FailureOption {
	return func(o *options) {
		go func() {
			<-ctx.Done()
			o.failureRate = 1
		}()
	}
}

// AfterNBytes will call effect once (roughly) n bytes have gone over the wire.
// Both sent and received bytes are counted against the total.  Only bytes in
// the body of an HTTP request are currently counted; this may change in the
// future.  effect will only be called once, and it will block (allowing
// callers to simulate connection hangs).
func AfterNBytes(n int, effect func()) FailureOption {
	return func(o *options) {
		o.trg = &triggerReaderGroup{
			bytes:   int64(n),
			trigger: effect,
		}
	}
}

type triggerReaderGroup struct {
	bytes     int64
	trigger   func()
	triggered int64
}

func (rg *triggerReaderGroup) new(rc io.ReadCloser) io.ReadCloser {
	return &triggerReader{
		ReadCloser: rc,
		bytes:      &rg.bytes,
		trigger:    rg.trigger,
		triggered:  &rg.triggered,
	}
}

type triggerReader struct {
	io.ReadCloser
	bytes     *int64
	trigger   func()
	triggered *int64
}

func (r *triggerReader) Read(p []byte) (int, error) {
	n, err := r.ReadCloser.Read(p)
	if atomic.AddInt64(r.bytes, -int64(n)) < 0 && atomic.CompareAndSwapInt64(r.triggered, 0, 1) {
		// Can't use sync.Once because it blocks for *all* callers until Do returns.
		r.trigger()
	}
	return n, err
}
