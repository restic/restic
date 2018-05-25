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

package window

import (
	"testing"
	"time"
)

type epair struct {
	e interface{}
	t time.Time
}

func adder(i, j interface{}) interface{} {
	a, ok := i.(int)
	if !ok {
		a = 0
	}
	b, ok := j.(int)
	if !ok {
		b = 0
	}
	return a + b
}

func TestWindows(t *testing.T) {
	table := []struct {
		size, dur time.Duration
		incs      []epair
		look      time.Time
		reduce    Reducer
		want      interface{}
	}{
		{
			size: time.Minute,
			dur:  time.Second,
			incs: []epair{
				// year, month, day, hour, min, sec, nano
				{t: time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC), e: 1},
				{t: time.Date(2000, 1, 1, 0, 0, 1, 0, time.UTC), e: 1},
				{t: time.Date(2000, 1, 1, 0, 0, 2, 0, time.UTC), e: 1},
				{t: time.Date(2000, 1, 1, 0, 0, 3, 0, time.UTC), e: 1},
				{t: time.Date(2000, 1, 1, 0, 0, 4, 0, time.UTC), e: 1},
				{t: time.Date(2000, 1, 1, 0, 0, 5, 0, time.UTC), e: 1},
			},
			look:   time.Date(2000, 1, 1, 0, 1, 0, 0, time.UTC),
			want:   5,
			reduce: adder,
		},
		{
			incs: []epair{
				// year, month, day, hour, min, sec, nano
				{t: time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC), e: 1},
				{t: time.Date(2000, 1, 1, 0, 0, 1, 0, time.UTC), e: 1},
				{t: time.Date(2000, 1, 1, 0, 0, 2, 0, time.UTC), e: 1},
				{t: time.Date(2000, 1, 1, 0, 0, 3, 0, time.UTC), e: 1},
				{t: time.Date(2000, 1, 1, 0, 0, 4, 0, time.UTC), e: 1},
				{t: time.Date(2000, 1, 1, 0, 0, 5, 0, time.UTC), e: 1},
			},
			want:   6,
			reduce: adder,
		},
		{ // what happens if time goes backwards?
			size: time.Minute,
			dur:  time.Second,
			incs: []epair{
				{t: time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC), e: 1},
				{t: time.Date(2000, 1, 1, 0, 0, 1, 0, time.UTC), e: 1},
				{t: time.Date(2000, 1, 1, 0, 0, 2, 0, time.UTC), e: 1},
				{t: time.Date(2000, 1, 1, 0, 0, 3, 0, time.UTC), e: 1},
				{t: time.Date(2000, 1, 1, 0, 0, 4, 0, time.UTC), e: 1},
				{t: time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC), e: 1},
			},
			look:   time.Date(2000, 1, 1, 0, 0, 30, 0, time.UTC),
			want:   1,
			reduce: adder,
		},
	}

	for _, e := range table {
		w := New(e.size, e.dur, e.reduce)
		for _, inc := range e.incs {
			w.insertAt(inc.t, inc.e)
		}
		ct := w.reducedAt(e.look)
		if ct != e.want {
			t.Errorf("reducedAt(%v) got %v, want %v", e.look, ct, e.want)
		}
	}
}
