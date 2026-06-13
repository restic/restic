package main

import (
	"testing"

	rtest "github.com/restic/restic/internal/test"
)

func TestSizeHistogramNew(t *testing.T) {
	h := newSizeHistogram(42)

	exp := &sizeHistogram{
		count:     0,
		totalSize: 0,
		buckets: []sizeClass{
			{0, 0, 0},
			{1, 9, 0},
			{10, 42, 0},
		},
	}

	rtest.Equals(t, exp, h)
}

func TestSizeHistogramAdd(t *testing.T) {
	h := newSizeHistogram(42)
	for i := uint64(0); i < 45; i++ {
		h.Add(i)
	}

	exp := &sizeHistogram{
		count:     45,
		totalSize: 990,
		buckets: []sizeClass{
			{0, 0, 1},
			{1, 9, 9},
			{10, 42, 33},
		},
		oversized: []uint64{43, 44},
	}

	rtest.Equals(t, exp, h)
}

func TestSizeHistogramString(t *testing.T) {
	t.Run("overflow", func(t *testing.T) {
		h := newSizeHistogram(42)
		h.Add(8)
		h.Add(50)

		rtest.Equals(t, "Count: 2\nTotal Size: 58 B\nSize        Count\n-----------------\n1 - 9 Byte  1\n-----------------\nOversized: [50]\n", h.String())
	})

	t.Run("withZero", func(t *testing.T) {
		h := newSizeHistogram(42)
		h.Add(0)
		h.Add(1)
		h.Add(10)

		rtest.Equals(t, "Count: 3\nTotal Size: 11 B\nSize          Count\n-------------------\n  0 - 0 Byte  1\n  1 - 9 Byte  1\n10 - 42 Byte  1\n-------------------\n", h.String())
	})
}
