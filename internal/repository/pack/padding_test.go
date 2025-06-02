package pack

import (
	"testing"

	"github.com/klauspost/compress/zstd"

	rtest "github.com/restic/restic/internal/test"
)

func TestPadmé(t *testing.T) {
	for _, c := range []struct {
		size, exp uint
	}{
		{0, 0},
		{1, 0},
		// Expected sizes computed using the Python reference implementation.
		{2, 0},
		{3, 0},
		{4, 0},
		{5, 0},
		{7, 0},
		{8, 0},
		{9, 1},
		{10, 0},
		{11, 1},
		{15, 1},
		{16, 0},
		{17, 1},
		{19, 1},
		{100, 4},
		{1000, 24},
		{1024, 0},
		{12345, 455},
		{31337, 407},
		{65536, 0},
		{65537, 2047},
		{16777215, 1},
		{16777216, 0},
		{16777217, 524287},
		{16777218, 524286},
		{16777219, 524285},
		{16777316, 524188},
		{33554432, 0},
		{33554433, 1048575},
		{33554435, 1048573},
		{4294967295, 1},
		//{1099511627676, 100},
	} {
		rtest.Equals(t, c.exp, padmé(c.size))
	}
}

func TestSkippableFrame(t *testing.T) {
	dec, err := zstd.NewReader(nil)
	rtest.OK(t, err)

	for _, size := range []uint32{0, 1, 3, 6, 8, 9, 20, 100, 1<<16 - 3, 1<<18 + 4} {
		f := skippableFrame(size)
		p, err := dec.DecodeAll(f, nil)
		rtest.OK(t, err)
		rtest.Equals(t, 0, len(p))
	}
}
