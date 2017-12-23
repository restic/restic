// Copyright 2017, The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE.md file.

package value

import (
	"bytes"
	"io"
	"reflect"
	"testing"
)

func TestFormat(t *testing.T) {
	type key struct {
		a int
		b string
		c chan bool
	}

	tests := []struct {
		in   interface{}
		want string
	}{{
		in:   []int{},
		want: "[]int{}",
	}, {
		in:   []int(nil),
		want: "[]int(nil)",
	}, {
		in:   []int{1, 2, 3, 4, 5},
		want: "[]int{1, 2, 3, 4, 5}",
	}, {
		in:   []interface{}{1, true, "hello", struct{ A, B int }{1, 2}},
		want: "[]interface {}{1, true, \"hello\", struct { A int; B int }{A: 1, B: 2}}",
	}, {
		in:   []struct{ A, B int }{{1, 2}, {0, 4}, {}},
		want: "[]struct { A int; B int }{{A: 1, B: 2}, {B: 4}, {}}",
	}, {
		in:   map[*int]string{new(int): "hello"},
		want: "map[*int]string{0x00: \"hello\"}",
	}, {
		in:   map[key]string{{}: "hello"},
		want: "map[value.key]string{{}: \"hello\"}",
	}, {
		in:   map[key]string{{a: 5, b: "key", c: make(chan bool)}: "hello"},
		want: "map[value.key]string{{a: 5, b: \"key\", c: (chan bool)(0x00)}: \"hello\"}",
	}, {
		in:   map[io.Reader]string{new(bytes.Reader): "hello"},
		want: "map[io.Reader]string{0x00: \"hello\"}",
	}, {
		in: func() interface{} {
			var a = []interface{}{nil}
			a[0] = a
			return a
		}(),
		want: "[]interface {}{([]interface {})(0x00)}",
	}, {
		in: func() interface{} {
			type A *A
			var a A
			a = &a
			return a
		}(),
		want: "&(value.A)(0x00)",
	}, {
		in: func() interface{} {
			type A map[*A]A
			a := make(A)
			a[&a] = a
			return a
		}(),
		want: "value.A{0x00: 0x00}",
	}, {
		in: func() interface{} {
			var a [2]interface{}
			a[0] = &a
			return a
		}(),
		want: "[2]interface {}{&[2]interface {}{(*[2]interface {})(0x00), interface {}(nil)}, interface {}(nil)}",
	}}

	formatFakePointers = true
	defer func() { formatFakePointers = false }()
	for i, tt := range tests {
		got := Format(reflect.ValueOf(tt.in), true)
		if got != tt.want {
			t.Errorf("test %d, Format():\ngot  %q\nwant %q", i, got, tt.want)
		}
	}
}
