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

package window_test

import (
	"fmt"
	"time"

	"github.com/kurin/blazer/x/window"
)

type Accumulator struct {
	w *window.Window
}

func (a Accumulator) Add(s string) {
	a.w.Insert([]string{s})
}

func (a Accumulator) All() []string {
	v := a.w.Reduce()
	return v.([]string)
}

func NewAccum(size time.Duration) Accumulator {
	r := func(i, j interface{}) interface{} {
		a, ok := i.([]string)
		if !ok {
			a = nil
		}
		b, ok := j.([]string)
		if !ok {
			b = nil
		}
		for _, s := range b {
			a = append(a, s)
		}
		return a
	}
	return Accumulator{w: window.New(size, time.Second, r)}
}

func Example_accumulator() {
	a := NewAccum(time.Minute)
	a.Add("this")
	a.Add("is")
	a.Add("that")
	fmt.Printf("total: %v\n", a.All())
	// Output:
	// total: [this is that]
}
