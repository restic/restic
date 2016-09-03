package chunker

import (
	"strconv"
	"testing"
)

var polAddTests = []struct {
	x, y Pol
	sum  Pol
}{
	{23, 16, 23 ^ 16},
	{0x9a7e30d1e855e0a0, 0x670102a1f4bcd414, 0xfd7f32701ce934b4},
	{0x9a7e30d1e855e0a0, 0x9a7e30d1e855e0a0, 0},
}

func TestPolAdd(t *testing.T) {
	for i, test := range polAddTests {
		if test.sum != test.x.Add(test.y) {
			t.Errorf("test %d failed: sum != x+y", i)
		}

		if test.sum != test.y.Add(test.x) {
			t.Errorf("test %d failed: sum != y+x", i)
		}
	}
}

func parseBin(s string) Pol {
	i, err := strconv.ParseUint(s, 2, 64)
	if err != nil {
		panic(err)
	}

	return Pol(i)
}

var polMulTests = []struct {
	x, y Pol
	res  Pol
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
		if test.res != m {
			t.Errorf("TestPolMul failed for test %d: %v * %v: want %v, got %v",
				i, test.x, test.y, test.res, m)
		}
		m = test.y.Mul(test.x)
		if test.res != test.y.Mul(test.x) {
			t.Errorf("TestPolMul failed for %d: %v * %v: want %v, got %v",
				i, test.x, test.y, test.res, m)
		}
	}
}

func TestPolMulOverflow(t *testing.T) {
	defer func() {
		// try to recover overflow error
		err := recover()

		if e, ok := err.(string); ok && e == "multiplication would overflow uint64" {
			return
		}

		t.Logf("invalid error raised: %v", err)
		// re-raise error if not overflow
		panic(err)
	}()

	x := Pol(1 << 63)
	x.Mul(2)
	t.Fatal("overflow test did not panic")
}

var polDivTests = []struct {
	x, y Pol
	res  Pol
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
		if test.res != m {
			t.Errorf("TestPolDiv failed for test %d: %v * %v: want %v, got %v",
				i, test.x, test.y, test.res, m)
		}
	}
}

func TestPolDeg(t *testing.T) {
	var x Pol
	if x.Deg() != -1 {
		t.Errorf("deg(0) is not -1: %v", x.Deg())
	}

	x = 1
	if x.Deg() != 0 {
		t.Errorf("deg(1) is not 0: %v", x.Deg())
	}

	for i := 0; i < 64; i++ {
		x = 1 << uint(i)
		if x.Deg() != i {
			t.Errorf("deg(1<<%d) is not %d: %v", i, i, x.Deg())
		}
	}
}

var polModTests = []struct {
	x, y Pol
	res  Pol
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
	for i, test := range polModTests {
		res := test.x.Mod(test.y)
		if test.res != res {
			t.Errorf("test %d failed: want %v, got %v", i, test.res, res)
		}
	}
}

func BenchmarkPolDivMod(t *testing.B) {
	f := Pol(0x2482734cacca49)
	g := Pol(0x3af4b284899)

	for i := 0; i < t.N; i++ {
		g.DivMod(f)
	}
}

func BenchmarkPolDiv(t *testing.B) {
	f := Pol(0x2482734cacca49)
	g := Pol(0x3af4b284899)

	for i := 0; i < t.N; i++ {
		g.Div(f)
	}
}

func BenchmarkPolMod(t *testing.B) {
	f := Pol(0x2482734cacca49)
	g := Pol(0x3af4b284899)

	for i := 0; i < t.N; i++ {
		g.Mod(f)
	}
}

func BenchmarkPolDeg(t *testing.B) {
	f := Pol(0x3af4b284899)
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
	_, err := RandomPolynomial()
	if err != nil {
		t.Fatal(err)
	}
}

func BenchmarkRandomPolynomial(t *testing.B) {
	for i := 0; i < t.N; i++ {
		_, err := RandomPolynomial()
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestExpandPolynomial(t *testing.T) {
	pol := Pol(0x3DA3358B4DC173)
	s := pol.Expand()
	if s != "x^53+x^52+x^51+x^50+x^48+x^47+x^45+x^41+x^40+x^37+x^36+x^34+x^32+x^31+x^27+x^25+x^24+x^22+x^19+x^18+x^16+x^15+x^14+x^8+x^6+x^5+x^4+x+1" {
		t.Fatal("wrong result")
	}
}

var polIrredTests = []struct {
	f     Pol
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
		if test.f.Irreducible() != test.irred {
			t.Errorf("Irreducibility test for Polynomial %v failed: got %v, wanted %v",
				test.f, test.f.Irreducible(), test.irred)
		}
	}
}

func BenchmarkPolIrreducible(b *testing.B) {
	// find first irreducible polynomial
	var pol Pol
	for _, test := range polIrredTests {
		if test.irred {
			pol = test.f
			break
		}
	}

	for i := 0; i < b.N; i++ {
		if !pol.Irreducible() {
			b.Errorf("Irreducibility test for Polynomial %v failed", pol)
		}
	}
}

var polGCDTests = []struct {
	f1  Pol
	f2  Pol
	gcd Pol
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
		if test.gcd != gcd {
			t.Errorf("GCD test %d (%+v) failed: got %v, wanted %v",
				i, test, gcd, test.gcd)
		}

		gcd = test.f2.GCD(test.f1)
		if test.gcd != gcd {
			t.Errorf("GCD test %d (%+v) failed: got %v, wanted %v",
				i, test, gcd, test.gcd)
		}
	}
}

var polMulModTests = []struct {
	f1  Pol
	f2  Pol
	g   Pol
	mod Pol
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
		if mod != test.mod {
			t.Errorf("MulMod test %d (%+v) failed: got %v, wanted %v",
				i, test, mod, test.mod)
		}
	}
}
