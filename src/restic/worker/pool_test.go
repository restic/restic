package worker_test

import (
	"testing"

	"restic/errors"

	"restic/worker"
)

const concurrency = 10

var errTooLarge = errors.New("too large")

func square(job worker.Job, done <-chan struct{}) (interface{}, error) {
	n := job.Data.(int)
	if n > 2000 {
		return nil, errTooLarge
	}
	return n * n, nil
}

func newBufferedPool(bufsize int, n int, f worker.Func) (chan worker.Job, chan worker.Job, *worker.Pool) {
	inCh := make(chan worker.Job, bufsize)
	outCh := make(chan worker.Job, bufsize)

	return inCh, outCh, worker.New(n, f, inCh, outCh)
}

func TestPool(t *testing.T) {
	inCh, outCh, p := newBufferedPool(200, concurrency, square)

	for i := 0; i < 150; i++ {
		inCh <- worker.Job{Data: i}
	}

	close(inCh)
	p.Wait()

	for res := range outCh {
		if res.Error != nil {
			t.Errorf("unexpected error for job %v received: %v", res.Data, res.Error)
			continue
		}

		n := res.Data.(int)
		m := res.Result.(int)

		if m != n*n {
			t.Errorf("wrong value for job %d returned: want %d, got %d", n, n*n, m)
		}
	}
}

func TestPoolErrors(t *testing.T) {
	inCh, outCh, p := newBufferedPool(200, concurrency, square)

	for i := 0; i < 150; i++ {
		inCh <- worker.Job{Data: i + 1900}
	}

	close(inCh)
	p.Wait()

	for res := range outCh {
		n := res.Data.(int)

		if n > 2000 {
			if res.Error == nil {
				t.Errorf("expected error not found, result is %v", res)
				continue
			}

			if res.Error != errTooLarge {
				t.Errorf("unexpected error found, result is %v", res)
			}

			continue
		} else {
			if res.Error != nil {
				t.Errorf("unexpected error for job %v received: %v", res.Data, res.Error)
				continue
			}
		}

		m := res.Result.(int)
		if m != n*n {
			t.Errorf("wrong value for job %d returned: want %d, got %d", n, n*n, m)
		}
	}
}
