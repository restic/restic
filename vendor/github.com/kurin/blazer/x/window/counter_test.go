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

type Counter struct {
	w *window.Window
}

func (c Counter) Add() {
	c.w.Insert(1)
}

func (c Counter) Count() int {
	v := c.w.Reduce()
	return v.(int)
}

func New(size time.Duration) Counter {
	r := func(i, j interface{}) interface{} {
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
	return Counter{w: window.New(size, time.Second, r)}
}

func Example_counter() {
	c := New(time.Minute)
	c.Add()
	c.Add()
	c.Add()
	fmt.Printf("total: %d\n", c.Count())
	// Output:
	// total: 3
}
