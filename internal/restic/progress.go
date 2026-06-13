package restic

// Counter tracks progress for long-running operations.
type Counter interface {
	Add(uint64)
	SetMax(uint64)
	Get() (uint64, uint64)
	Done()
}

type noopCounter struct{}

func (noopCounter) Add(uint64)            {}
func (noopCounter) SetMax(uint64)         {}
func (noopCounter) Get() (uint64, uint64) { return 0, 0 }
func (noopCounter) Done()                 {}

// NoopCounter is a Counter that discards all updates.
var NoopCounter Counter = noopCounter{}

type noopTerminalCounterFactory struct{}

func (noopTerminalCounterFactory) NewCounterTerminalOnly(string) Counter {
	return NoopCounter
}

// NoopTerminalCounterFactory is a TerminalCounterFactory that returns NoopCounter.
var NoopTerminalCounterFactory TerminalCounterFactory = noopTerminalCounterFactory{}
