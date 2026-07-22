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

// Printer can return a new counter or print messages at different log levels.
// It must be safe to call its methods from concurrent goroutines.
type Printer interface {
	// NewCounter returns a new progress counter. It is not shown if --quiet or --json is specified.
	NewCounter(description string) Counter
	// NewCounterTerminalOnly returns a new progress counter that is only shown if stdout points to a
	// terminal. It is not shown if --quiet or --json is specified.
	NewCounterTerminalOnly(description string) Counter

	// E reports an error. This message is always printed to stderr.
	// Appends a newline if not present.
	E(msg string, args ...any)
	// S prints a message, this is should only be used for very important messages
	// that are not errors. The message is even printed if --quiet is specified.
	// Appends a newline if not present.
	S(msg string, args ...any)
	// PT prints a message if verbosity >= 1  (neither --quiet nor --verbose is specified)
	// and stdout points to a terminal.
	// This is used for informational messages.
	PT(msg string, args ...any)
	// P prints a message if verbosity >= 1 (neither --quiet nor --verbose is specified),
	// this is used for normal messages which are not errors. Appends a newline if not present.
	P(msg string, args ...any)
	// V prints a message if verbosity >= 2 (equivalent to --verbose), this is used for
	// verbose messages. Appends a newline if not present.
	V(msg string, args ...any)
	// VV prints a message if verbosity >= 3 (equivalent to --verbose=2), this is used for
	// debug messages. Appends a newline if not present.
	VV(msg string, args ...any)
}

// noopPrinter discards all messages.
type noopPrinter struct{}

var (
	_ Printer                = (*noopPrinter)(nil)
	_ TerminalCounterFactory = (*noopPrinter)(nil)
)

// NewNoopPrinter returns a Printer that discards all messages.
func NewNoopPrinter() Printer {
	return &noopPrinter{}
}

func (*noopPrinter) NewCounter(_ string) Counter {
	return NoopCounter
}

func (*noopPrinter) NewCounterTerminalOnly(_ string) Counter {
	return NoopCounter
}

func (*noopPrinter) E(_ string, _ ...any) {}

func (*noopPrinter) S(_ string, _ ...any) {}

func (*noopPrinter) PT(_ string, _ ...any) {}

func (*noopPrinter) P(_ string, _ ...any) {}

func (*noopPrinter) V(_ string, _ ...any) {}

func (*noopPrinter) VV(_ string, _ ...any) {}
