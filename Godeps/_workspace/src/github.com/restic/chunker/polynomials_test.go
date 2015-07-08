package chunker_test

import (
	"strconv"
	"testing"

	"github.com/restic/chunker"
	. "github.com/restic/restic/test"
)

var polAddTests = []struct {
	x, y chunker.Pol
	sum  chunker.Pol
}{
	{23, 16, 23 ^ 16},
	{0x9a7e30d1e855e0a0, 0x670102a1f4bcd414, 0xfd7f32701ce934b4},
	{0x9a7e30d1e855e0a0, 0x9a7e30d1e855e0a0, 0},
}

func TestPolAdd(t *testing.T) {
	for _, test := range polAddTests {
		Equals(t, test.sum, test.x.Add(test.y))
		Equals(t, test.sum, test.y.Add(test.x))
	}
}

func parseBin(s string) chunker.Pol {
	i, err := strconv.ParseUint(s, 2, 64)
	if err != nil {
		panic(err)
	}

	return chunker.Pol(i)
}

var polMulTests = []struct {
	x, y chunker.Pol
	res  chunker.Pol
}{
	{1, 2, 2},
	{
		parseBin("1101"),
		parseBin("10"),
		parseBin("11010"),
	},
	{
		parseBin("1101"),
		parseBin("11"),
		parseBin("10111"),
	},
	{
		0x40000000,
		0x40000000,
		0x1000000000000000,
	},
	{
		parseBin("1010"),
		parseBin("100100"),
		parseBin("101101000"),
	},
	{
		parseBin("100"),
		parseBin("11"),
		parseBin("1100"),
	},
	{
		parseBin("11"),
		parseBin("110101"),
		parseBin("1011111"),
	},
	{
		parseBin("10011"),
		parseBin("110101"),
		parseBin("1100001111"),
	},
}

func TestPolMul(t *testing.T) {
	for i, test := range polMulTests {
		m := test.x.Mul(test.y)
		Assert(t, test.res == m,
			"TestPolMul failed for test %d: %v * %v: want %v, got %v",
			i, test.x, test.y, test.res, m)
		m = test.y.Mul(test.x)
		Assert(t, test.res == test.y.Mul(test.x),
			"TestPolMul failed for %d: %v * %v: want %v, got %v",
			i, test.x, test.y, test.res, m)
	}
}

func TestPolMulOverflow(t *testing.T) {
	defer func() {
		// try to recover overflow error
		err := recover()

		if e, ok := err.(string); ok && e == "multiplication would overflow uint64" {
			return
		} else {
			t.Logf("invalid error raised: %v", err)
			// re-raise error if not overflow
			panic(err)
		}
	}()

	x := chunker.Pol(1 << 63)
	x.Mul(2)
	t.Fatal("overflow test did not panic")
}

var polDivTests = []struct {
	x, y chunker.Pol
	res  chunker.Pol
}{
	{10, 50, 0},
	{0, 1, 0},
	{
		parseBin("101101000"), // 0x168
		parseBin("1010"),      // 0xa
		parseBin("100100"),    // 0x24
	},
	{2, 2, 1},
	{
		0x8000000000000000,
		0x8000000000000000,
		1,
	},
	{
		parseBin("1100"),
		parseBin("100"),
		parseBin("11"),
	},
	{
		parseBin("1100001111"),
		parseBin("10011"),
		parseBin("110101"),
	},
}

func TestPolDiv(t *testing.T) {
	for i, test := range polDivTests {
		m := test.x.Div(test.y)
		Assert(t, test.res == m,
			"TestPolDiv failed for test %d: %v * %v: want %v, got %v",
			i, test.x, test.y, test.res, m)
	}
}

var polModTests = []struct {
	x, y chunker.Pol
	res  chunker.Pol
}{
	{10, 50, 10},
	{0, 1, 0},
	{
		parseBin("101101001"),
		parseBin("1010"),
		parseBin("1"),
	},
	{2, 2, 0},
	{
		0x8000000000000000,
		0x8000000000000000,
		0,
	},
	{
		parseBin("1100"),
		parseBin("100"),
		parseBin("0"),
	},
	{
		parseBin("1100001111"),
		parseBin("10011"),
		parseBin("0"),
	},
}

func TestPolModt(t *testing.T) {
	for _, test := range polModTests {
		Equals(t, test.res, test.x.Mod(test.y))
	}
}

func BenchmarkPolDivMod(t *testing.B) {
	f := chunker.Pol(0x2482734cacca49)
	g := chunker.Pol(0x3af4b284899)

	for i := 0; i < t.N; i++ {
		g.DivMod(f)
	}
}

func BenchmarkPolDiv(t *testing.B) {
	f := chunker.Pol(0x2482734cacca49)
	g := chunker.Pol(0x3af4b284899)

	for i := 0; i < t.N; i++ {
		g.Div(f)
	}
}

func BenchmarkPolMod(t *testing.B) {
	f := chunker.Pol(0x2482734cacca49)
	g := chunker.Pol(0x3af4b284899)

	for i := 0; i < t.N; i++ {
		g.Mod(f)
	}
}

func BenchmarkPolDeg(t *testing.B) {
	f := chunker.Pol(0x3af4b284899)
	d := f.Deg()
	if d != 41 {
		t.Fatalf("BenchmalPolDeg: Wrong degree %d returned, expected %d",
			d, 41)
	}

	for i := 0; i < t.N; i++ {
		f.Deg()
	}
}

func TestRandomPolynomial(t *testing.T) {
	_, err := chunker.RandomPolynomial()
	OK(t, err)
}

func BenchmarkRandomPolynomial(t *testing.B) {
	for i := 0; i < t.N; i++ {
		_, err := chunker.RandomPolynomial()
		OK(t, err)
	}
}

func TestExpandPolynomial(t *testing.T) {
	pol := chunker.Pol(0x3DA3358B4DC173)
	s := pol.Expand()
	Equals(t, "x^53+x^52+x^51+x^50+x^48+x^47+x^45+x^41+x^40+x^37+x^36+x^34+x^32+x^31+x^27+x^25+x^24+x^22+x^19+x^18+x^16+x^15+x^14+x^8+x^6+x^5+x^4+x+1", s)
}

var polIrredTests = []struct {
	f     chunker.Pol
	irred bool
}{
	{0x38f1e565e288df, false},
	{0x3DA3358B4DC173, true},
	{0x30a8295b9d5c91, false},
	{0x255f4350b962cb, false},
	{0x267f776110a235, false},
	{0x2f4dae10d41227, false},
	{0x2482734cacca49, true},
	{0x312daf4b284899, false},
	{0x29dfb6553d01d1, false},
	{0x3548245eb26257, false},
	{0x3199e7ef4211b3, false},
	{0x362f39017dae8b, false},
	{0x200d57aa6fdacb, false},
	{0x35e0a4efa1d275, false},
	{0x2ced55b026577f, false},
	{0x260b012010893d, false},
	{0x2df29cbcd59e9d, false},
	{0x3f2ac7488bd429, false},
	{0x3e5cb1711669fb, false},
	{0x226d8de57a9959, false},
	{0x3c8de80aaf5835, false},
	{0x2026a59efb219b, false},
	{0x39dfa4d13fb231, false},
	{0x3143d0464b3299, false},
}

func TestPolIrreducible(t *testing.T) {
	for _, test := range polIrredTests {
		Assert(t, test.f.Irreducible() == test.irred,
			"Irreducibility test for Polynomial %v failed: got %v, wanted %v",
			test.f, test.f.Irreducible(), test.irred)
	}
}

func BenchmarkPolIrreducible(b *testing.B) {
	// find first irreducible polynomial
	var pol chunker.Pol
	for _, test := range polIrredTests {
		if test.irred {
			pol = test.f
			break
		}
	}

	for i := 0; i < b.N; i++ {
		Assert(b, pol.Irreducible(),
			"Irreducibility test for Polynomial %v failed", pol)
	}
}

var polGCDTests = []struct {
	f1  chunker.Pol
	f2  chunker.Pol
	gcd chunker.Pol
}{
	{10, 50, 2},
	{0, 1, 1},
	{
		parseBin("101101001"),
		parseBin("1010"),
		parseBin("1"),
	},
	{2, 2, 2},
	{
		parseBin("1010"),
		parseBin("11"),
		parseBin("11"),
	},
	{
		0x8000000000000000,
		0x8000000000000000,
		0x8000000000000000,
	},
	{
		parseBin("1100"),
		parseBin("101"),
		parseBin("11"),
	},
	{
		parseBin("1100001111"),
		parseBin("10011"),
		parseBin("10011"),
	},
	{
		0x3DA3358B4DC173,
		0x3DA3358B4DC173,
		0x3DA3358B4DC173,
	},
	{
		0x3DA3358B4DC173,
		0x230d2259defd,
		1,
	},
	{
		0x230d2259defd,
		0x51b492b3eff2,
		parseBin("10011"),
	},
}

func TestPolGCD(t *testing.T) {
	for i, test := range polGCDTests {
		gcd := test.f1.GCD(test.f2)
		Assert(t, test.gcd == gcd,
			"GCD test %d (%+v) failed: got %v, wanted %v",
			i, test, gcd, test.gcd)
		gcd = test.f2.GCD(test.f1)
		Assert(t, test.gcd == gcd,
			"GCD test %d (%+v) failed: got %v, wanted %v",
			i, test, gcd, test.gcd)
	}
}

var polMulModTests = []struct {
	f1  chunker.Pol
	f2  chunker.Pol
	g   chunker.Pol
	mod chunker.Pol
}{
	{
		0x1230,
		0x230,
		0x55,
		0x22,
	},
	{
		0x0eae8c07dbbb3026,
		0xd5d6db9de04771de,
		0xdd2bda3b77c9,
		0x425ae8595b7a,
	},
}

func TestPolMulMod(t *testing.T) {
	for i, test := range polMulModTests {
		mod := test.f1.MulMod(test.f2, test.g)
		Assert(t, mod == test.mod,
			"MulMod test %d (%+v) failed: got %v, wanted %v",
			i, test, mod, test.mod)
	}
}
