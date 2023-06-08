package backup

import (
	"fmt"
	"math"
	"testing"
	"time"

	rtest "github.com/restic/restic/internal/test"
)

const float64EqualityThreshold = 1e-6

func almostEqual(a, b float64) bool {
	if math.IsNaN(a) || math.IsNaN(b) {
		panic("almostEqual passed a NaN")
	}
	return math.Abs(a-b) <= float64EqualityThreshold
}

func TestEstimatorDefault(t *testing.T) {
	var start time.Time
	e := newRateEstimator(start)
	r := e.rate(start)
	rtest.Assert(t, r == 0, "e.Rate == %v, want zero", r)
	r = e.rate(start.Add(time.Hour))
	rtest.Assert(t, r == 0, "e.Rate == %v, want zero", r)
}

func TestEstimatorSimple(t *testing.T) {
	var start time.Time
	type testcase struct {
		bytes uint64
		when  time.Duration
		rate  float64
	}

	cases := []testcase{
		{0, 0, 0},
		{1, time.Second, 1},
		{60, time.Second, 60},
		{60, time.Minute, 1},
	}
	for _, c := range cases {
		name := fmt.Sprintf("%+v", c)
		t.Run(name, func(t *testing.T) {
			e := newRateEstimator(start)
			e.recordBytes(start.Add(time.Second), c.bytes)
			rate := e.rate(start.Add(c.when))
			rtest.Assert(t, almostEqual(rate, c.rate), "e.Rate == %v, want %v", rate, c.rate)
		})
	}
}

func TestBucketWidth(t *testing.T) {
	var when time.Time

	// Recording byte transfers within a bucket width's time window uses one
	// bucket.
	e := newRateEstimator(when)
	e.recordBytes(when, 1)
	e.recordBytes(when.Add(bucketWidth-time.Nanosecond), 1)
	rtest.Assert(t, e.buckets.Len() == 1, "e.buckets.Len() is %d, want 1", e.buckets.Len())

	b := e.buckets.Back().Value.(*rateBucket)
	rtest.Assert(t, b.totalBytes == 2, "b.totalBytes is %d, want 2", b.totalBytes)
	rtest.Assert(t, b.end == when.Add(bucketWidth), "b.end is %v, want %v", b.end, when.Add(bucketWidth))

	// Recording a byte outside the bucket width causes another bucket.
	e.recordBytes(when.Add(bucketWidth), 1)
	rtest.Assert(t, e.buckets.Len() == 2, "e.buckets.Len() is %d, want 2", e.buckets.Len())

	b = e.buckets.Back().Value.(*rateBucket)
	rtest.Assert(t, b.totalBytes == 1, "b.totalBytes is %d, want 1", b.totalBytes)
	rtest.Assert(t, b.end == when.Add(2*bucketWidth), "b.end is %v, want %v", b.end, when.Add(bucketWidth))

	// Recording a byte after a longer delay creates a sparse bucket list.
	e.recordBytes(when.Add(time.Hour+time.Millisecond), 7)
	rtest.Assert(t, e.buckets.Len() == 3, "e.buckets.Len() is %d, want 3", e.buckets.Len())

	b = e.buckets.Back().Value.(*rateBucket)
	rtest.Assert(t, b.totalBytes == 7, "b.totalBytes is %d, want 7", b.totalBytes)
	rtest.Equals(t, when.Add(time.Hour+time.Millisecond+time.Second), b.end)
}

type chunk struct {
	repetitions uint64 // repetition count
	bytes       uint64 // byte count (every second)
}

func applyChunks(chunks []chunk, t time.Time, e *rateEstimator) time.Time {
	for _, c := range chunks {
		for i := uint64(0); i < c.repetitions; i++ {
			e.recordBytes(t, c.bytes)
			t = t.Add(time.Second)
		}
	}
	return t
}

func TestEstimatorResponsiveness(t *testing.T) {
	type testcase struct {
		description string
		chunks      []chunk
		rate        float64
	}

	cases := []testcase{
		{
			"1000 bytes/sec over one second",
			[]chunk{
				{1, 1000},
			},
			1000,
		},
		{
			"1000 bytes/sec over one minute",
			[]chunk{
				{60, 1000},
			},
			1000,
		},
		{
			"1000 bytes/sec for 10 seconds, then 2000 bytes/sec for 10 seconds",
			[]chunk{
				{10, 1000},
				{10, 2000},
			},
			1500,
		},
		{
			"1000 bytes/sec for one minute, then 2000 bytes/sec for one minute",
			[]chunk{
				{60, 1000},
				{60, 2000},
			},
			1500,
		},
		{
			"rate doubles after 30 seconds",
			[]chunk{
				{30, minRateEstimatorBytes},
				{90, 2 * minRateEstimatorBytes},
			},
			minRateEstimatorBytes * 1.75,
		},
		{
			"rate doubles after 31 seconds",
			[]chunk{
				{31, minRateEstimatorBytes},
				{90, 2 * minRateEstimatorBytes},
			},
			// The expected rate is the same as the prior test case because the
			// first second has rolled off the estimator.
			minRateEstimatorBytes * 1.75,
		},
		{
			"rate doubles after 90 seconds",
			[]chunk{
				{90, minRateEstimatorBytes},
				{90, 2 * minRateEstimatorBytes},
			},
			// The expected rate is the same as the prior test case because the
			// first 60 seconds have rolled off the estimator.
			minRateEstimatorBytes * 1.75,
		},
		{
			"rate doubles for two full minutes",
			[]chunk{
				{60, minRateEstimatorBytes},
				{120, 2 * minRateEstimatorBytes},
			},
			2 * minRateEstimatorBytes,
		},
		{
			"rate falls to zero",
			[]chunk{
				{30, minRateEstimatorBytes},
				{30, 0},
			},
			minRateEstimatorBytes / 2,
		},
		{
			"rate falls to zero for extended time",
			[]chunk{
				{60, 1000},
				{300, 0},
			},
			1000 * 60 / (60 + 300.0),
		},
		{
			"rate falls to zero for extended time (from high rate)",
			[]chunk{
				{2 * minRateEstimatorBuckets, minRateEstimatorBytes},
				{300, 0},
			},
			// Expect that only minRateEstimatorBuckets buckets are used in the
			// rate estimate.
			minRateEstimatorBytes * minRateEstimatorBuckets /
				(minRateEstimatorBuckets + 300.0),
		},
	}

	for _, c := range cases {
		t.Run(c.description, func(t *testing.T) {
			var w time.Time
			e := newRateEstimator(w)
			w = applyChunks(c.chunks, w, e)
			r := e.rate(w)
			rtest.Assert(t, almostEqual(r, c.rate), "e.Rate == %f, want %f", r, c.rate)
		})
	}
}
