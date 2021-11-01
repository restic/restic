package backup

import (
	"math"
	"testing"
	"time"
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
	if math.IsNaN(r) || r != 0 {
		t.Fatalf("e.Rate == %v, want zero", r)
	}
	r = e.rate(start.Add(time.Hour))
	if math.IsNaN(r) || r != 0 {
		t.Fatalf("e.Rate == %v, want zero", r)
	}
}

func TestEstimatorSimple(t *testing.T) {
	var when time.Time
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
		e := newRateEstimator(when)
		e.recordBytes(when.Add(time.Second), c.bytes)
		rate := e.rate(when.Add(c.when))
		if !almostEqual(rate, c.rate) {
			t.Fatalf("e.Rate == %v, want %v (testcase %+v)", rate, c.rate, c)
		}
	}
}

func TestBucketWidth(t *testing.T) {
	var when time.Time

	// Recording byte transfers within a bucket width's time window uses one
	// bucket.
	e := newRateEstimator(when)
	e.recordBytes(when, 1)
	e.recordBytes(when.Add(bucketWidth-time.Nanosecond), 1)
	if e.buckets.Len() != 1 {
		t.Fatalf("e.buckets.Len() is %d, want 1", e.buckets.Len())
	}
	b := e.buckets.Back().Value.(*rateBucket)
	if b.totalBytes != 2 {
		t.Fatalf("b.totalBytes is %d, want 2", b.totalBytes)
	}
	if b.end != when.Add(bucketWidth) {
		t.Fatalf("b.end is %v, want %v", b.end, when.Add(bucketWidth))
	}

	// Recording a byte outside the bucket width causes another bucket.
	e.recordBytes(when.Add(bucketWidth), 1)
	if e.buckets.Len() != 2 {
		t.Fatalf("e.buckets.Len() is %d, want 2", e.buckets.Len())
	}
	b = e.buckets.Back().Value.(*rateBucket)
	if b.totalBytes != 1 {
		t.Fatalf("b.totalBytes is %d, want 1", b.totalBytes)
	}
	if b.end != when.Add(2*bucketWidth) {
		t.Fatalf("b.end is %v, want %v", b.end, when.Add(bucketWidth))
	}

	// Recording a byte after a longer delay creates a sparse bucket list.
	e.recordBytes(when.Add(time.Hour+time.Millisecond), 7)
	if e.buckets.Len() != 3 {
		t.Fatalf("e.buckets.Len() is %d, want 3", e.buckets.Len())
	}
	b = e.buckets.Back().Value.(*rateBucket)
	if b.totalBytes != 7 {
		t.Fatalf("b.totalBytes is %d, want 7", b.totalBytes)
	}
	if b.end != when.Add(time.Hour+time.Millisecond+time.Second) {
		t.Fatalf("b.end is %v, want %v", b.end, when.Add(time.Hour+time.Millisecond+time.Second))
	}
}

type chunk struct {
	repetitions uint64 // repetition count
	bytes       uint64 // byte count (every second)
}

func applyChunk(c chunk, t time.Time, e *rateEstimator) time.Time {
	for i := uint64(0); i < c.repetitions; i++ {
		e.recordBytes(t, c.bytes)
		t = t.Add(time.Second)
	}
	return t
}

func applyChunks(chunks []chunk, t time.Time, e *rateEstimator) time.Time {
	for _, c := range chunks {
		t = applyChunk(c, t, e)
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

	for i, c := range cases {
		var w time.Time
		e := newRateEstimator(w)
		w = applyChunks(c.chunks, w, e)
		r := e.rate(w)
		if !almostEqual(r, c.rate) {
			t.Fatalf("e.Rate == %f, want %f (testcase %d %+v)", r, c.rate, i, c)
		}
	}
}
