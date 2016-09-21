package worker

// Job is one unit of work. It is given to a Func, and the returned result and
// error are stored in Result and Error.
type Job struct {
	Data   interface{}
	Result interface{}
	Error  error
}

// Func does the actual work within a Pool.
type Func func(job Job, done <-chan struct{}) (result interface{}, err error)

// Pool implements a worker pool.
type Pool struct {
	f     Func
	done  chan struct{}
	jobCh <-chan Job
	resCh chan<- Job

	numWorkers     int
	workersExit    chan struct{}
	allWorkersDone chan struct{}
}

// New returns a new worker pool with n goroutines, each running the function
// f. The workers are started immediately.
func New(n int, f Func, jobChan <-chan Job, resultChan chan<- Job) *Pool {
	p := &Pool{
		f:              f,
		done:           make(chan struct{}),
		workersExit:    make(chan struct{}),
		allWorkersDone: make(chan struct{}),
		numWorkers:     n,
		jobCh:          jobChan,
		resCh:          resultChan,
	}

	for i := 0; i < n; i++ {
		go p.runWorker(i)
	}

	go p.waitForExit()

	return p
}

// waitForExit receives from p.workersExit until all worker functions have
// exited, then closes the result channel.
func (p *Pool) waitForExit() {
	n := p.numWorkers
	for n > 0 {
		<-p.workersExit
		n--
	}
	close(p.allWorkersDone)
	close(p.resCh)
}

// runWorker runs a worker function.
func (p *Pool) runWorker(numWorker int) {
	defer func() {
		p.workersExit <- struct{}{}
	}()

	var (
		// enable the input channel when starting up a new goroutine
		inCh = p.jobCh
		// but do not enable the output channel until we have a result
		outCh chan<- Job

		job Job
		ok  bool
	)

	for {
		select {
		case <-p.done:
			return

		case job, ok = <-inCh:
			if !ok {
				return
			}

			job.Result, job.Error = p.f(job, p.done)
			inCh = nil
			outCh = p.resCh

		case outCh <- job:
			outCh = nil
			inCh = p.jobCh
		}
	}
}

// Wait waits for all worker goroutines to terminate, afterwards the output
// channel is closed.
func (p *Pool) Wait() {
	<-p.allWorkersDone
}
