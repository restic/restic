package scrypt

import (
	"fmt"
	"reflect"
	"testing"
	"time"
)

// Test cases
var (
	testLengths = []int{1, 8, 16, 32, 100, 500, 2500}
	password    = "super-secret-password"
)

var testParams = []struct {
	pass   bool
	params Params
}{
	{true, Params{16384, 8, 1, 32, 64}},
	{true, Params{16384, 8, 1, 16, 32}},
	{true, Params{65536, 8, 1, 16, 64}},
	{true, Params{1048576, 8, 2, 64, 128}},
	{false, Params{-1, 8, 1, 16, 32}},          // invalid N
	{false, Params{0, 8, 1, 16, 32}},           // invalid N
	{false, Params{1<<31 - 1, 8, 1, 16, 32}},   // invalid N
	{false, Params{16384, 0, 12, 16, 32}},      // invalid R
	{false, Params{16384, 8, 0, 16, 32}},       // invalid R > maxInt/128/P
	{false, Params{16384, 1 << 24, 1, 16, 32}}, // invalid R > maxInt/256
	{false, Params{1<<31 - 1, 8, 0, 16, 32}},   // invalid p < 0
	{false, Params{4096, 8, 1, 5, 32}},         // invalid SaltLen
	{false, Params{4096, 8, 1, 16, 2}},         // invalid DKLen
}

var testHashes = []struct {
	pass bool
	hash string
}{
	{false, "1$8$1$9003d0e8e69482843e6bd560c2c9cd94$1976f233124e0ee32bb2678eb1b0ed668eb66cff6fa43279d1e33f6e81af893b"},          // N too small
	{false, "$9003d0e8e69482843e6bd560c2c9cd94$1976f233124e0ee32bb2678eb1b0ed668eb66cff6fa43279d1e33f6e81af893b"},               // too short
	{false, "16384#8#1#18fbc325efa37402d27c3c2172900cbf$d4e5e1b9eedc1a6a14aad6624ab57b7b42ae75b9c9845fde32de765835f2aaf9"},      // incorrect separators
	{false, "16384$nogood$1$18fbc325efa37402d27c3c2172900cbf$d4e5e1b9eedc1a6a14aad6624ab57b7b42ae75b9c9845fde32de765835f2aaf9"}, // invalid R
	{false, "16384$8$abc1$18fbc325efa37402d27c3c2172900cbf$d4e5e1b9eedc1a6a14aad6624ab57b7b42ae75b9c9845fde32de765835f2aaf9"},   // invalid P
	{false, "16384$8$1$Tk9QRQ==$d4e5e1b9eedc1a6a14aad6624ab57b7b42ae75b9c9845fde32de765835f2aaf9"},                              // invalid salt (not hex)
	{false, "16384$8$1$18fbc325efa37402d27c3c2172900cbf$42ae====/75b9c9845fde32de765835f2aaf9"},                                 // invalid dk (not hex)
}

func TestGenerateRandomBytes(t *testing.T) {
	for _, v := range testLengths {
		_, err := GenerateRandomBytes(v)
		if err != nil {
			t.Fatalf("failed to generate random bytes")
		}
	}
}

func TestGenerateFromPassword(t *testing.T) {
	for _, v := range testParams {
		_, err := GenerateFromPassword([]byte(password), v.params)
		if err != nil && v.pass == true {
			t.Fatalf("no error was returned when expected for params: %+v", v.params)
		}
	}
}

func TestCompareHashAndPassword(t *testing.T) {
	hash, err := GenerateFromPassword([]byte(password), DefaultParams)
	if err != nil {
		t.Fatal(err)
	}

	if err := CompareHashAndPassword(hash, []byte(password)); err != nil {
		t.Fatal(err)
	}

	if err := CompareHashAndPassword(hash, []byte("invalid-password")); err == nil {
		t.Fatalf("mismatched passwords did not produce an error")
	}

	invalidHash := []byte("$166$$11$a2ad56a415af5")
	if err := CompareHashAndPassword(invalidHash, []byte(password)); err == nil {
		t.Fatalf("did not identify an invalid hash")
	}

}

func TestCost(t *testing.T) {
	hash, err := GenerateFromPassword([]byte(password), DefaultParams)
	if err != nil {
		t.Fatal(err)
	}

	params, err := Cost(hash)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(params, DefaultParams) {
		t.Fatal("cost mismatch: parameters used did not match those retrieved")
	}
}

func TestDecodeHash(t *testing.T) {
	for _, v := range testHashes {
		_, err := Cost([]byte(v.hash))
		if err == nil && v.pass == false {
			t.Fatal("invalid hash: did not correctly detect invalid password hash")
		}
	}
}

func TestCalibrate(t *testing.T) {
	timeout := 500 * time.Millisecond
	for testNum, tc := range []struct {
		MemMiB int
	}{
		{64},
		{32},
		{16},
		{8},
		{1},
	} {
		var (
			p   Params
			err error
		)
		p, err = Calibrate(timeout, tc.MemMiB, p)
		if err != nil {
			t.Fatalf("%d. %#v: %v", testNum, p, err)
		}
		if (128*p.R*p.N)>>20 > tc.MemMiB {
			t.Errorf("%d. wanted memory limit %d, got %d.", testNum, tc.MemMiB, (128*p.R*p.N)>>20)
		}
		start := time.Now()
		_, err = GenerateFromPassword([]byte(password), p)
		dur := time.Since(start)
		t.Logf("GenerateFromPassword with %#v took %s (%v)", p, dur, err)
		if err != nil {
			t.Fatalf("%d. GenerateFromPassword with %#v: %v", testNum, p, err)
		}
		if dur < timeout/2 {
			t.Errorf("%d. GenerateFromPassword was too fast (wanted around %s, got %s) with %#v.", testNum, timeout, dur, p)
		} else if timeout*2 < dur {
			t.Errorf("%d. GenerateFromPassword took too long (wanted around %s, got %s) with %#v.", testNum, timeout, dur, p)
		}
	}
}

func ExampleCalibrate() {
	p, err := Calibrate(1*time.Second, 128, Params{})
	if err != nil {
		panic(err)
	}
	dk, err := GenerateFromPassword([]byte("super-secret-password"), p)
	fmt.Printf("generated password is %q (%v)", dk, err)
}
