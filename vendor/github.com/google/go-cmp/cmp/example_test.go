// Copyright 2017, The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE.md file.

package cmp_test

import (
	"fmt"
	"math"
	"reflect"
	"sort"
	"strings"

	"github.com/google/go-cmp/cmp"
)

// TODO: Re-write these examples in terms of how you actually use the
// fundamental options and filters and not in terms of what cool things you can
// do with them since that overlaps with cmp/cmpopts.

// Use Diff for printing out human-readable errors for test cases comparing
// nested or structured data.
func ExampleDiff_testing() {
	// Code under test:
	type ShipManifest struct {
		Name     string
		Crew     map[string]string
		Androids int
		Stolen   bool
	}

	// AddCrew tries to add the given crewmember to the manifest.
	AddCrew := func(m *ShipManifest, name, title string) {
		if m.Crew == nil {
			m.Crew = make(map[string]string)
		}
		m.Crew[title] = name
	}

	// Test function:
	tests := []struct {
		desc        string
		before      *ShipManifest
		name, title string
		after       *ShipManifest
	}{
		{
			desc:   "add to empty",
			before: &ShipManifest{},
			name:   "Zaphod Beeblebrox",
			title:  "Galactic President",
			after: &ShipManifest{
				Crew: map[string]string{
					"Zaphod Beeblebrox": "Galactic President",
				},
			},
		},
		{
			desc: "add another",
			before: &ShipManifest{
				Crew: map[string]string{
					"Zaphod Beeblebrox": "Galactic President",
				},
			},
			name:  "Trillian",
			title: "Human",
			after: &ShipManifest{
				Crew: map[string]string{
					"Zaphod Beeblebrox": "Galactic President",
					"Trillian":          "Human",
				},
			},
		},
		{
			desc: "overwrite",
			before: &ShipManifest{
				Crew: map[string]string{
					"Zaphod Beeblebrox": "Galactic President",
				},
			},
			name:  "Zaphod Beeblebrox",
			title: "Just this guy, you know?",
			after: &ShipManifest{
				Crew: map[string]string{
					"Zaphod Beeblebrox": "Just this guy, you know?",
				},
			},
		},
	}

	var t fakeT
	for _, test := range tests {
		AddCrew(test.before, test.name, test.title)
		if diff := cmp.Diff(test.before, test.after); diff != "" {
			t.Errorf("%s: after AddCrew, manifest differs: (-got +want)\n%s", test.desc, diff)
		}
	}

	// Output:
	// add to empty: after AddCrew, manifest differs: (-got +want)
	// {*cmp_test.ShipManifest}.Crew["Galactic President"]:
	// 	-: "Zaphod Beeblebrox"
	// 	+: <non-existent>
	// {*cmp_test.ShipManifest}.Crew["Zaphod Beeblebrox"]:
	// 	-: <non-existent>
	// 	+: "Galactic President"
	//
	// add another: after AddCrew, manifest differs: (-got +want)
	// {*cmp_test.ShipManifest}.Crew["Human"]:
	// 	-: "Trillian"
	// 	+: <non-existent>
	// {*cmp_test.ShipManifest}.Crew["Trillian"]:
	// 	-: <non-existent>
	// 	+: "Human"
	//
	// overwrite: after AddCrew, manifest differs: (-got +want)
	// {*cmp_test.ShipManifest}.Crew["Just this guy, you know?"]:
	// 	-: "Zaphod Beeblebrox"
	// 	+: <non-existent>
	// {*cmp_test.ShipManifest}.Crew["Zaphod Beeblebrox"]:
	// 	-: "Galactic President"
	// 	+: "Just this guy, you know?"
}

// Approximate equality for floats can be handled by defining a custom
// comparer on floats that determines two values to be equal if they are within
// some range of each other.
//
// This example is for demonstrative purposes; use cmpopts.EquateApprox instead.
func ExampleOption_approximateFloats() {
	// This Comparer only operates on float64.
	// To handle float32s, either define a similar function for that type
	// or use a Transformer to convert float32s into float64s.
	opt := cmp.Comparer(func(x, y float64) bool {
		delta := math.Abs(x - y)
		mean := math.Abs(x+y) / 2.0
		return delta/mean < 0.00001
	})

	x := []float64{1.0, 1.1, 1.2, math.Pi}
	y := []float64{1.0, 1.1, 1.2, 3.14159265359} // Accurate enough to Pi
	z := []float64{1.0, 1.1, 1.2, 3.1415}        // Diverges too far from Pi

	fmt.Println(cmp.Equal(x, y, opt))
	fmt.Println(cmp.Equal(y, z, opt))
	fmt.Println(cmp.Equal(z, x, opt))

	// Output:
	// true
	// false
	// false
}

// Normal floating-point arithmetic defines == to be false when comparing
// NaN with itself. In certain cases, this is not the desired property.
//
// This example is for demonstrative purposes; use cmpopts.EquateNaNs instead.
func ExampleOption_equalNaNs() {
	// This Comparer only operates on float64.
	// To handle float32s, either define a similar function for that type
	// or use a Transformer to convert float32s into float64s.
	opt := cmp.Comparer(func(x, y float64) bool {
		return (math.IsNaN(x) && math.IsNaN(y)) || x == y
	})

	x := []float64{1.0, math.NaN(), math.E, -0.0, +0.0}
	y := []float64{1.0, math.NaN(), math.E, -0.0, +0.0}
	z := []float64{1.0, math.NaN(), math.Pi, -0.0, +0.0} // Pi constant instead of E

	fmt.Println(cmp.Equal(x, y, opt))
	fmt.Println(cmp.Equal(y, z, opt))
	fmt.Println(cmp.Equal(z, x, opt))

	// Output:
	// true
	// false
	// false
}

// To have floating-point comparisons combine both properties of NaN being
// equal to itself and also approximate equality of values, filters are needed
// to restrict the scope of the comparison so that they are composable.
//
// This example is for demonstrative purposes;
// use cmpopts.EquateNaNs and cmpopts.EquateApprox instead.
func ExampleOption_equalNaNsAndApproximateFloats() {
	alwaysEqual := cmp.Comparer(func(_, _ interface{}) bool { return true })

	opts := cmp.Options{
		// This option declares that a float64 comparison is equal only if
		// both inputs are NaN.
		cmp.FilterValues(func(x, y float64) bool {
			return math.IsNaN(x) && math.IsNaN(y)
		}, alwaysEqual),

		// This option declares approximate equality on float64s only if
		// both inputs are not NaN.
		cmp.FilterValues(func(x, y float64) bool {
			return !math.IsNaN(x) && !math.IsNaN(y)
		}, cmp.Comparer(func(x, y float64) bool {
			delta := math.Abs(x - y)
			mean := math.Abs(x+y) / 2.0
			return delta/mean < 0.00001
		})),
	}

	x := []float64{math.NaN(), 1.0, 1.1, 1.2, math.Pi}
	y := []float64{math.NaN(), 1.0, 1.1, 1.2, 3.14159265359} // Accurate enough to Pi
	z := []float64{math.NaN(), 1.0, 1.1, 1.2, 3.1415}        // Diverges too far from Pi

	fmt.Println(cmp.Equal(x, y, opts))
	fmt.Println(cmp.Equal(y, z, opts))
	fmt.Println(cmp.Equal(z, x, opts))

	// Output:
	// true
	// false
	// false
}

// Sometimes, an empty map or slice is considered equal to an allocated one
// of zero length.
//
// This example is for demonstrative purposes; use cmpopts.EquateEmpty instead.
func ExampleOption_equalEmpty() {
	alwaysEqual := cmp.Comparer(func(_, _ interface{}) bool { return true })

	// This option handles slices and maps of any type.
	opt := cmp.FilterValues(func(x, y interface{}) bool {
		vx, vy := reflect.ValueOf(x), reflect.ValueOf(y)
		return (vx.IsValid() && vy.IsValid() && vx.Type() == vy.Type()) &&
			(vx.Kind() == reflect.Slice || vx.Kind() == reflect.Map) &&
			(vx.Len() == 0 && vy.Len() == 0)
	}, alwaysEqual)

	type S struct {
		A []int
		B map[string]bool
	}
	x := S{nil, make(map[string]bool, 100)}
	y := S{make([]int, 0, 200), nil}
	z := S{[]int{0}, nil} // []int has a single element (i.e., not empty)

	fmt.Println(cmp.Equal(x, y, opt))
	fmt.Println(cmp.Equal(y, z, opt))
	fmt.Println(cmp.Equal(z, x, opt))

	// Output:
	// true
	// false
	// false
}

// Two slices may be considered equal if they have the same elements,
// regardless of the order that they appear in. Transformations can be used
// to sort the slice.
//
// This example is for demonstrative purposes; use cmpopts.SortSlices instead.
func ExampleOption_sortedSlice() {
	// This Transformer sorts a []int.
	// Since the transformer transforms []int into []int, there is problem where
	// this is recursively applied forever. To prevent this, use a FilterValues
	// to first check for the condition upon which the transformer ought to apply.
	trans := cmp.FilterValues(func(x, y []int) bool {
		return !sort.IntsAreSorted(x) || !sort.IntsAreSorted(y)
	}, cmp.Transformer("Sort", func(in []int) []int {
		out := append([]int(nil), in...) // Copy input to avoid mutating it
		sort.Ints(out)
		return out
	}))

	x := struct{ Ints []int }{[]int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}}
	y := struct{ Ints []int }{[]int{2, 8, 0, 9, 6, 1, 4, 7, 3, 5}}
	z := struct{ Ints []int }{[]int{0, 0, 1, 2, 3, 4, 5, 6, 7, 8}}

	fmt.Println(cmp.Equal(x, y, trans))
	fmt.Println(cmp.Equal(y, z, trans))
	fmt.Println(cmp.Equal(z, x, trans))

	// Output:
	// true
	// false
	// false
}

type otherString string

func (x otherString) Equal(y otherString) bool {
	return strings.ToLower(string(x)) == strings.ToLower(string(y))
}

// If the Equal method defined on a type is not suitable, the type can be be
// dynamically transformed to be stripped of the Equal method (or any method
// for that matter).
func ExampleOption_avoidEqualMethod() {
	// Suppose otherString.Equal performs a case-insensitive equality,
	// which is too loose for our needs.
	// We can avoid the methods of otherString by declaring a new type.
	type myString otherString

	// This transformer converts otherString to myString, allowing Equal to use
	// other Options to determine equality.
	trans := cmp.Transformer("", func(in otherString) myString {
		return myString(in)
	})

	x := []otherString{"foo", "bar", "baz"}
	y := []otherString{"fOO", "bAr", "Baz"} // Same as before, but with different case

	fmt.Println(cmp.Equal(x, y))        // Equal because of case-insensitivity
	fmt.Println(cmp.Equal(x, y, trans)) // Not equal because of more exact equality

	// Output:
	// true
	// false
}

func roundF64(z float64) float64 {
	if z < 0 {
		return math.Ceil(z - 0.5)
	}
	return math.Floor(z + 0.5)
}

// The complex numbers complex64 and complex128 can really just be decomposed
// into a pair of float32 or float64 values. It would be convenient to be able
// define only a single comparator on float64 and have float32, complex64, and
// complex128 all be able to use that comparator. Transformations can be used
// to handle this.
func ExampleOption_transformComplex() {
	opts := []cmp.Option{
		// This transformer decomposes complex128 into a pair of float64s.
		cmp.Transformer("T1", func(in complex128) (out struct{ Real, Imag float64 }) {
			out.Real, out.Imag = real(in), imag(in)
			return out
		}),
		// This transformer converts complex64 to complex128 to allow the
		// above transform to take effect.
		cmp.Transformer("T2", func(in complex64) complex128 {
			return complex128(in)
		}),
		// This transformer converts float32 to float64.
		cmp.Transformer("T3", func(in float32) float64 {
			return float64(in)
		}),
		// This equality function compares float64s as rounded integers.
		cmp.Comparer(func(x, y float64) bool {
			return roundF64(x) == roundF64(y)
		}),
	}

	x := []interface{}{
		complex128(3.0), complex64(5.1 + 2.9i), float32(-1.2), float64(12.3),
	}
	y := []interface{}{
		complex128(3.1), complex64(4.9 + 3.1i), float32(-1.3), float64(11.7),
	}
	z := []interface{}{
		complex128(3.8), complex64(4.9 + 3.1i), float32(-1.3), float64(11.7),
	}

	fmt.Println(cmp.Equal(x, y, opts...))
	fmt.Println(cmp.Equal(y, z, opts...))
	fmt.Println(cmp.Equal(z, x, opts...))

	// Output:
	// true
	// false
	// false
}

type fakeT struct{}

func (t fakeT) Errorf(format string, args ...interface{}) { fmt.Printf(format+"\n", args...) }
