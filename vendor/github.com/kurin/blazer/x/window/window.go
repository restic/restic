// Copyright 2018, Google
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

// Package window provides a type for efficiently recording events as they
// occur over a given span of time.  Events added to the window will remain
// until the time expires.
package window

import (
	"sync"
	"time"
)

// A Window efficiently records events that have occurred over a span of time
// extending from some fixed interval ago to now.  Events that pass beyond this
// horizon are discarded.
type Window struct {
	mu      sync.Mutex
	events  []interface{}
	res     time.Duration
	last    time.Time
	reduce  Reducer
	forever bool
	e       interface{}
}

// A Reducer should take two values from the window and combine them into a
// third value that will be stored in the window.  The values i or j may be
// nil.  The underlying types for both arguments and the output should be
// identical.
//
// If the reducer is any kind of slice or list, then data usage will grow
// linearly with the number of events added to the window.
//
// Reducer will be called on its own output: Reducer(Reducer(x, y), z).
type Reducer func(i, j interface{}) interface{}

// New returns an initialized window for events over the given duration at the
// given resolution.  Windows with tight resolution (i.e., small values for
// that argument) will be more accurate, at the cost of some memory.
//
// A size of 0 means "forever"; old events will never be removed.
func New(size, resolution time.Duration, r Reducer) *Window {
	if size > 0 {
		return &Window{
			res:    resolution,
			events: make([]interface{}, size/resolution),
			reduce: r,
		}
	}
	return &Window{
		forever: true,
		reduce:  r,
	}
}

func (w *Window) bucket(now time.Time) int {
	nanos := now.UnixNano()
	abs := nanos / int64(w.res)
	return int(abs) % len(w.events)
}

// sweep keeps the window valid.  It needs to be called from every method that
// views or updates the window, and the caller needs to hold the mutex.
func (w *Window) sweep(now time.Time) {
	if w.forever {
		return
	}
	defer func() {
		w.last = now
	}()

	// This compares now and w.last's monotonic clocks.
	diff := now.Sub(w.last)
	if diff < 0 {
		// time went backwards somehow; zero events and return
		for i := range w.events {
			w.events[i] = nil
		}
		return
	}
	last := now.Add(-diff)

	b := w.bucket(now)
	p := w.bucket(last)

	if b == p && diff <= w.res {
		// We're in the same bucket as the previous sweep, so all buckets are
		// valid.
		return
	}

	if diff > w.res*time.Duration(len(w.events)) {
		// We've gone longer than this window measures since the last sweep, just
		// zero the thing and have done.
		for i := range w.events {
			w.events[i] = nil
		}
		return
	}

	// Expire all invalid buckets.  This means buckets not seen since the
	// previous sweep and now, including the current bucket but not including the
	// previous bucket.
	old := int64(last.UnixNano()) / int64(w.res)
	new := int64(now.UnixNano()) / int64(w.res)
	for i := old + 1; i <= new; i++ {
		b := int(i) % len(w.events)
		w.events[b] = nil
	}
}

// Insert adds the given event.
func (w *Window) Insert(e interface{}) {
	w.insertAt(time.Now(), e)
}

func (w *Window) insertAt(t time.Time, e interface{}) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.forever {
		w.e = w.reduce(w.e, e)
		return
	}

	w.sweep(t)
	w.events[w.bucket(t)] = w.reduce(w.events[w.bucket(t)], e)
}

// Reduce runs the window's reducer over the valid values and returns the
// result.
func (w *Window) Reduce() interface{} {
	return w.reducedAt(time.Now())
}

func (w *Window) reducedAt(t time.Time) interface{} {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.forever {
		return w.e
	}

	w.sweep(t)
	var n interface{}
	for i := range w.events {
		n = w.reduce(n, w.events[i])
	}
	return n
}
