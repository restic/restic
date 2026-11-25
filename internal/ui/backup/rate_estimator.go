package backup

import (
	"container/list"
	"time"
)

// rateBucket represents a one second window of recorded progress.
type rateBucket struct {
	totalBytes uint64
	end        time.Time // the end of the time window, exclusive
}

// rateEstimator represents an estimate of the time to complete an operation.
type rateEstimator struct {
	buckets    *list.List
	start      time.Time
	totalBytes uint64
}

// newRateEstimator returns an estimator initialized to a presumed start time.
func newRateEstimator(start time.Time) *rateEstimator {
	return &rateEstimator{buckets: list.New(), start: start}
}

// See trim(), below.
const (
	bucketWidth             = time.Second
	minRateEstimatorBytes   = 100 * 1000 * 1000
	minRateEstimatorBuckets = 20
	minRateEstimatorMinutes = 2
)

// currentRateWindow is the trailing time window used to compute the
// "current" throughput shown in backup progress output.
const currentRateWindow = 15 * time.Second

// trim removes the oldest history from the estimator assuming a given
// current time.
func (r *rateEstimator) trim(now time.Time) {
	// The estimator retains byte transfer counts over a two minute window.
	// However, to avoid removing too much history when transfer rates are
	// low, the estimator also retains a minimum number of processed bytes
	// across a minimum number of buckets. An operation that is processing a
	// significant number of bytes per second will typically retain only a
	// two minute window's worth of information. One that is making slow
	// progress, such as one being over a rate limited connection, typically
	// observes bursts of updates as infrequently as every ten or twenty
	// seconds, in which case the other limiters will kick in. This heuristic
	// avoids wildly fluctuating estimates over rate limited connections.
	start := now.Add(-minRateEstimatorMinutes * time.Minute)

	for e := r.buckets.Front(); e != nil; e = r.buckets.Front() {
		if r.buckets.Len() <= minRateEstimatorBuckets {
			break
		}
		b := e.Value.(*rateBucket)
		if b.end.After(start) {
			break
		}
		total := r.totalBytes - b.totalBytes
		if total < minRateEstimatorBytes {
			break
		}
		r.start = b.end
		r.totalBytes = total
		r.buckets.Remove(e)
	}
}

// recordBytes records the transfer of a number of bytes at a given
// time. Times passed in successive calls should advance monotonically (as
// is the case with time.Now().
func (r *rateEstimator) recordBytes(now time.Time, bytes uint64) {
	if bytes == 0 {
		return
	}
	var tail *rateBucket
	if r.buckets.Len() > 0 {
		tail = r.buckets.Back().Value.(*rateBucket)
	}
	if tail == nil || !tail.end.After(now) {
		// The new bucket holds measurements in the time range [now .. now+1sec).
		tail = &rateBucket{end: now.Add(bucketWidth)}
		r.buckets.PushBack(tail)
	}
	tail.totalBytes += bytes
	r.totalBytes += bytes
	r.trim(now)
}

// rate returns an estimated bytes per second rate at a given time, or zero
// if there is not enough data to compute a rate.
func (r *rateEstimator) rate(now time.Time) float64 {
	r.trim(now)
	if !r.start.Before(now) {
		return 0
	}
	elapsed := float64(now.Sub(r.start)) / float64(time.Second)
	rate := float64(r.totalBytes) / elapsed
	return rate
}

// windowRate returns the average bytes per second of recorded activity
// inside the trailing window ending at now. A bucket that only partially
// overlaps the front of the window contributes the proportional fraction
// of its bytes; the rate is divided by the actual time span covered.
func (r *rateEstimator) windowRate(now time.Time, window time.Duration) float64 {
	if window <= 0 || r.buckets.Len() == 0 {
		return 0
	}
	windowStart := now.Add(-window)
	var bytes uint64
	span := 0.0
	for e := r.buckets.Back(); e != nil; e = e.Prev() {
		b := e.Value.(*rateBucket)
		bucketStart := b.end.Add(-bucketWidth)
		if !b.end.After(windowStart) {
			break
		}
		if bucketStart.Before(windowStart) {
			covered := b.end.Sub(windowStart).Seconds()
			bytes += uint64(float64(b.totalBytes) * covered / bucketWidth.Seconds())
			span += covered
			break
		}
		bytes += b.totalBytes
		span += bucketWidth.Seconds()
	}
	if span == 0 {
		return 0
	}
	return float64(bytes) / span
}
