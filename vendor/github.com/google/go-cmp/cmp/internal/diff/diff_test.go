// Copyright 2017, The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE.md file.

package diff

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"
	"unicode"
)

func TestDifference(t *testing.T) {
	tests := []struct {
		// Before passing x and y to Difference, we strip all spaces so that
		// they can be used by the test author to indicate a missing symbol
		// in one of the lists.
		x, y string
		want string
	}{{
		x:    "",
		y:    "",
		want: "",
	}, {
		x:    "#",
		y:    "#",
		want: ".",
	}, {
		x:    "##",
		y:    "# ",
		want: ".X",
	}, {
		x:    "a#",
		y:    "A ",
		want: "MX",
	}, {
		x:    "#a",
		y:    " A",
		want: "XM",
	}, {
		x:    "# ",
		y:    "##",
		want: ".Y",
	}, {
		x:    " #",
		y:    "@#",
		want: "Y.",
	}, {
		x:    "@#",
		y:    " #",
		want: "X.",
	}, {
		x:    "##########0123456789",
		y:    "          0123456789",
		want: "XXXXXXXXXX..........",
	}, {
		x:    "          0123456789",
		y:    "##########0123456789",
		want: "YYYYYYYYYY..........",
	}, {
		x:    "#####0123456789#####",
		y:    "     0123456789     ",
		want: "XXXXX..........XXXXX",
	}, {
		x:    "     0123456789     ",
		y:    "#####0123456789#####",
		want: "YYYYY..........YYYYY",
	}, {
		x:    "01234##########56789",
		y:    "01234          56789",
		want: ".....XXXXXXXXXX.....",
	}, {
		x:    "01234          56789",
		y:    "01234##########56789",
		want: ".....YYYYYYYYYY.....",
	}, {
		x:    "0123456789##########",
		y:    "0123456789          ",
		want: "..........XXXXXXXXXX",
	}, {
		x:    "0123456789          ",
		y:    "0123456789##########",
		want: "..........YYYYYYYYYY",
	}, {
		x:    "abcdefghij0123456789",
		y:    "ABCDEFGHIJ0123456789",
		want: "MMMMMMMMMM..........",
	}, {
		x:    "ABCDEFGHIJ0123456789",
		y:    "abcdefghij0123456789",
		want: "MMMMMMMMMM..........",
	}, {
		x:    "01234abcdefghij56789",
		y:    "01234ABCDEFGHIJ56789",
		want: ".....MMMMMMMMMM.....",
	}, {
		x:    "01234ABCDEFGHIJ56789",
		y:    "01234abcdefghij56789",
		want: ".....MMMMMMMMMM.....",
	}, {
		x:    "0123456789abcdefghij",
		y:    "0123456789ABCDEFGHIJ",
		want: "..........MMMMMMMMMM",
	}, {
		x:    "0123456789ABCDEFGHIJ",
		y:    "0123456789abcdefghij",
		want: "..........MMMMMMMMMM",
	}, {
		x:    "ABCDEFGHIJ0123456789          ",
		y:    "          0123456789abcdefghij",
		want: "XXXXXXXXXX..........YYYYYYYYYY",
	}, {
		x:    "          0123456789abcdefghij",
		y:    "ABCDEFGHIJ0123456789          ",
		want: "YYYYYYYYYY..........XXXXXXXXXX",
	}, {
		x:    "ABCDE0123456789     FGHIJ",
		y:    "     0123456789abcdefghij",
		want: "XXXXX..........YYYYYMMMMM",
	}, {
		x:    "     0123456789abcdefghij",
		y:    "ABCDE0123456789     FGHIJ",
		want: "YYYYY..........XXXXXMMMMM",
	}, {
		x:    "ABCDE01234F G H I J 56789     ",
		y:    "     01234 a b c d e56789fghij",
		want: "XXXXX.....XYXYXYXYXY.....YYYYY",
	}, {
		x:    "     01234a b c d e 56789fghij",
		y:    "ABCDE01234 F G H I J56789     ",
		want: "YYYYY.....XYXYXYXYXY.....XXXXX",
	}, {
		x:    "FGHIJ01234ABCDE56789     ",
		y:    "     01234abcde56789fghij",
		want: "XXXXX.....MMMMM.....YYYYY",
	}, {
		x:    "     01234abcde56789fghij",
		y:    "FGHIJ01234ABCDE56789     ",
		want: "YYYYY.....MMMMM.....XXXXX",
	}, {
		x:    "ABCAB BA ",
		y:    "  C BABAC",
		want: "XX.X.Y..Y",
	}, {
		x:    "# ####  ###",
		y:    "#y####yy###",
		want: ".Y....YY...",
	}, {
		x:    "# #### # ##x#x",
		y:    "#y####y y## # ",
		want: ".Y....YXY..X.X",
	}, {
		x:    "###z#z###### x  #",
		y:    "#y##Z#Z###### yy#",
		want: ".Y..M.M......XYY.",
	}, {
		x:    "0 12z3x 456789 x x 0",
		y:    "0y12Z3 y456789y y y0",
		want: ".Y..M.XY......YXYXY.",
	}, {
		x:    "0 2 4 6 8 ..................abXXcdEXF.ghXi",
		y:    " 1 3 5 7 9..................AB  CDE F.GH I",
		want: "XYXYXYXYXY..................MMXXMM.X..MMXM",
	}, {
		x:    "I HG.F EDC  BA..................9 7 5 3 1 ",
		y:    "iXhg.FXEdcXXba.................. 8 6 4 2 0",
		want: "MYMM..Y.MMYYMM..................XYXYXYXYXY",
	}, {
		x:    "x1234",
		y:    " 1234",
		want: "X....",
	}, {
		x:    "x123x4",
		y:    " 123 4",
		want: "X...X.",
	}, {
		x:    "x1234x56",
		y:    " 1234   ",
		want: "X....XXX",
	}, {
		x:    "x1234xxx56",
		y:    " 1234   56",
		want: "X....XXX..",
	}, {
		x:    ".1234...ab",
		y:    " 1234   AB",
		want: "X....XXXMM",
	}, {
		x:    "x1234xxab.",
		y:    " 1234  AB ",
		want: "X....XXMMX",
	}, {
		x:    " 0123456789",
		y:    "9012345678 ",
		want: "Y.........X",
	}, {
		x:    "  0123456789",
		y:    "8901234567  ",
		want: "YY........XX",
	}, {
		x:    "   0123456789",
		y:    "7890123456   ",
		want: "YYY.......XXX",
	}, {
		x:    "    0123456789",
		y:    "6789012345    ",
		want: "YYYY......XXXX",
	}, {
		x:    "0123456789     ",
		y:    "     5678901234",
		want: "XXXXX.....YYYYY",
	}, {
		x:    "0123456789    ",
		y:    "    4567890123",
		want: "XXXX......YYYY",
	}, {
		x:    "0123456789   ",
		y:    "   3456789012",
		want: "XXX.......YYY",
	}, {
		x:    "0123456789  ",
		y:    "  2345678901",
		want: "XX........YY",
	}, {
		x:    "0123456789 ",
		y:    " 1234567890",
		want: "X.........Y",
	}, {
		x: "0123456789",
		y: "9876543210",
	}, {
		x: "0123456789",
		y: "6725819034",
	}, {
		x: "FBQMOIGTLN72X90E4SP651HKRJUDA83CVZW",
		y: "5WHXO10R9IVKZLCTAJ8P3NSEQM472G6UBDF",
	}}

	for _, tt := range tests {
		tRun(t, "", func(t *testing.T) {
			x := strings.Replace(tt.x, " ", "", -1)
			y := strings.Replace(tt.y, " ", "", -1)
			es := testStrings(t, x, y)
			if got := es.String(); got != tt.want {
				t.Errorf("Difference(%s, %s):\ngot  %s\nwant %s", x, y, got, tt.want)
			}
		})
	}
}

func TestDifferenceFuzz(t *testing.T) {
	tests := []struct{ px, py, pm float32 }{
		{px: 0.0, py: 0.0, pm: 0.1},
		{px: 0.0, py: 0.1, pm: 0.0},
		{px: 0.1, py: 0.0, pm: 0.0},
		{px: 0.0, py: 0.1, pm: 0.1},
		{px: 0.1, py: 0.0, pm: 0.1},
		{px: 0.2, py: 0.2, pm: 0.2},
		{px: 0.3, py: 0.1, pm: 0.2},
		{px: 0.1, py: 0.3, pm: 0.2},
		{px: 0.2, py: 0.2, pm: 0.2},
		{px: 0.3, py: 0.3, pm: 0.3},
		{px: 0.1, py: 0.1, pm: 0.5},
		{px: 0.4, py: 0.1, pm: 0.5},
		{px: 0.3, py: 0.2, pm: 0.5},
		{px: 0.2, py: 0.3, pm: 0.5},
		{px: 0.1, py: 0.4, pm: 0.5},
	}

	for i, tt := range tests {
		tRun(t, fmt.Sprintf("P%d", i), func(t *testing.T) {
			// Sweep from 1B to 1KiB.
			for n := 1; n <= 1024; n <<= 1 {
				tRun(t, fmt.Sprintf("N%d", n), func(t *testing.T) {
					for j := 0; j < 10; j++ {
						x, y := generateStrings(n, tt.px, tt.py, tt.pm, int64(j))
						testStrings(t, x, y)
					}
				})
			}
		})
	}
}

func benchmarkDifference(b *testing.B, n int) {
	// TODO: Use testing.B.Run when we drop Go1.6 support.
	x, y := generateStrings(n, 0.05, 0.05, 0.10, 0)
	b.ReportAllocs()
	b.SetBytes(int64(len(x) + len(y)))
	for i := 0; i < b.N; i++ {
		Difference(len(x), len(y), func(ix, iy int) Result {
			return compareByte(x[ix], y[iy])
		})
	}
}
func BenchmarkDifference1K(b *testing.B)   { benchmarkDifference(b, 1<<10) }
func BenchmarkDifference4K(b *testing.B)   { benchmarkDifference(b, 1<<12) }
func BenchmarkDifference16K(b *testing.B)  { benchmarkDifference(b, 1<<14) }
func BenchmarkDifference64K(b *testing.B)  { benchmarkDifference(b, 1<<16) }
func BenchmarkDifference256K(b *testing.B) { benchmarkDifference(b, 1<<18) }
func BenchmarkDifference1M(b *testing.B)   { benchmarkDifference(b, 1<<20) }

func generateStrings(n int, px, py, pm float32, seed int64) (string, string) {
	if px+py+pm > 1.0 {
		panic("invalid probabilities")
	}
	py += px
	pm += py

	b := make([]byte, n)
	r := rand.New(rand.NewSource(seed))
	r.Read(b)

	var x, y []byte
	for len(b) > 0 {
		switch p := r.Float32(); {
		case p < px: // UniqueX
			x = append(x, b[0])
		case p < py: // UniqueY
			y = append(y, b[0])
		case p < pm: // Modified
			x = append(x, 'A'+(b[0]%26))
			y = append(y, 'a'+(b[0]%26))
		default: // Identity
			x = append(x, b[0])
			y = append(y, b[0])
		}
		b = b[1:]
	}
	return string(x), string(y)
}

func testStrings(t *testing.T, x, y string) EditScript {
	wantEq := x == y
	eq, es := Difference(len(x), len(y), func(ix, iy int) Result {
		return compareByte(x[ix], y[iy])
	})
	if eq != wantEq {
		t.Errorf("equality mismatch: got %v, want %v", eq, wantEq)
	}
	if es != nil {
		if es.LenX() != len(x) {
			t.Errorf("es.LenX = %d, want %d", es.LenX(), len(x))
		}
		if es.LenY() != len(y) {
			t.Errorf("es.LenY = %d, want %d", es.LenY(), len(y))
		}
		if got := (es.Dist() == 0); got != wantEq {
			t.Errorf("violation of equality invariant: got %v, want %v", got, wantEq)
		}
		if !validateScript(x, y, es) {
			t.Errorf("invalid edit script: %v", es)
		}
	}
	return es
}

func validateScript(x, y string, es EditScript) bool {
	var bx, by []byte
	for _, e := range es {
		switch e {
		case Identity:
			if !compareByte(x[len(bx)], y[len(by)]).Equal() {
				return false
			}
			bx = append(bx, x[len(bx)])
			by = append(by, y[len(by)])
		case UniqueX:
			bx = append(bx, x[len(bx)])
		case UniqueY:
			by = append(by, y[len(by)])
		case Modified:
			if !compareByte(x[len(bx)], y[len(by)]).Similar() {
				return false
			}
			bx = append(bx, x[len(bx)])
			by = append(by, y[len(by)])
		}
	}
	return string(bx) == x && string(by) == y
}

// compareByte returns a Result where the result is Equal if x == y,
// similar if x and y differ only in casing, and different otherwise.
func compareByte(x, y byte) (r Result) {
	switch {
	case x == y:
		return equalResult // Identity
	case unicode.ToUpper(rune(x)) == unicode.ToUpper(rune(y)):
		return similarResult // Modified
	default:
		return differentResult // UniqueX or UniqueY
	}
}

var (
	equalResult     = Result{NDiff: 0}
	similarResult   = Result{NDiff: 1}
	differentResult = Result{NDiff: 2}
)

func TestResult(t *testing.T) {
	tests := []struct {
		result      Result
		wantEqual   bool
		wantSimilar bool
	}{
		// equalResult is equal since NDiff == 0, by definition of Equal method.
		{equalResult, true, true},
		// similarResult is similar since it is a binary result where only one
		// element was compared (i.e., Either NSame==1 or NDiff==1).
		{similarResult, false, true},
		// differentResult is different since there are enough differences that
		// it isn't even considered similar.
		{differentResult, false, false},

		// Zero value is always equal.
		{Result{NSame: 0, NDiff: 0}, true, true},

		// Binary comparisons (where NSame+NDiff == 1) are always similar.
		{Result{NSame: 1, NDiff: 0}, true, true},
		{Result{NSame: 0, NDiff: 1}, false, true},

		// More complex ratios. The exact ratio for similarity may change,
		// and may require updates to these test cases.
		{Result{NSame: 1, NDiff: 1}, false, true},
		{Result{NSame: 1, NDiff: 2}, false, true},
		{Result{NSame: 1, NDiff: 3}, false, false},
		{Result{NSame: 2, NDiff: 1}, false, true},
		{Result{NSame: 2, NDiff: 2}, false, true},
		{Result{NSame: 2, NDiff: 3}, false, true},
		{Result{NSame: 3, NDiff: 1}, false, true},
		{Result{NSame: 3, NDiff: 2}, false, true},
		{Result{NSame: 3, NDiff: 3}, false, true},
		{Result{NSame: 1000, NDiff: 0}, true, true},
		{Result{NSame: 1000, NDiff: 1}, false, true},
		{Result{NSame: 1000, NDiff: 2}, false, true},
		{Result{NSame: 0, NDiff: 1000}, false, false},
		{Result{NSame: 1, NDiff: 1000}, false, false},
		{Result{NSame: 2, NDiff: 1000}, false, false},
	}

	for _, tt := range tests {
		if got := tt.result.Equal(); got != tt.wantEqual {
			t.Errorf("%#v.Equal() = %v, want %v", tt.result, got, tt.wantEqual)
		}
		if got := tt.result.Similar(); got != tt.wantSimilar {
			t.Errorf("%#v.Similar() = %v, want %v", tt.result, got, tt.wantSimilar)
		}
	}
}

// TODO: Delete this hack when we drop Go1.6 support.
func tRun(t *testing.T, name string, f func(t *testing.T)) {
	type runner interface {
		Run(string, func(t *testing.T)) bool
	}
	var ti interface{} = t
	if r, ok := ti.(runner); ok {
		r.Run(name, f)
	} else {
		t.Logf("Test: %s", name)
		f(t)
	}
}
