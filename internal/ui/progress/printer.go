package progress

import "testing"

// A Printer can can return a new counter or print messages
// at different log levels.
// It must be safe to call its methods from concurrent goroutines.
type Printer interface {
	// NewCounter returns a new progress counter. It is not shown if --quiet or --json is specified.
	NewCounter(description string) *Counter
	// NewCounterTerminalOnly returns a new progress counter that is only shown if stdout points to a
	// terminal. It is not shown if --quiet or --json is specified.
	NewCounterTerminalOnly(description string) *Counter

	// E reports an error. This message is always printed to stderr.
	// Appends a newline if not present.
	E(msg string, args ...interface{})
	// S prints a message, this is should only be used for very important messages
	// that are not errors. The message is even printed if --quiet is specified.
	// Appends a newline if not present.
	S(msg string, args ...interface{})
	// P prints a message if verbosity >= 1 (neither --quiet nor --verbose is specified),
	// this is used for normal messages which are not errors. Appends a newline if not present.
	P(msg string, args ...interface{})
	// V prints a message if verbosity >= 2 (equivalent to --verbose), this is used for
	// verbose messages. Appends a newline if not present.
	V(msg string, args ...interface{})
	// VV prints a message if verbosity >= 3 (equivalent to --verbose=2), this is used for
	// debug messages. Appends a newline if not present.
	VV(msg string, args ...interface{})
}

// NoopPrinter discards all messages
type NoopPrinter struct{}

var _ Printer = (*NoopPrinter)(nil)

func (*NoopPrinter) NewCounter(_ string) *Counter {
	return nil
}

func (*NoopPrinter) NewCounterTerminalOnly(_ string) *Counter {
	return nil
}

func (*NoopPrinter) E(_ string, _ ...interface{}) {}

func (*NoopPrinter) S(_ string, _ ...interface{}) {}

func (*NoopPrinter) P(_ string, _ ...interface{}) {}

func (*NoopPrinter) V(_ string, _ ...interface{}) {}

func (*NoopPrinter) VV(_ string, _ ...interface{}) {}

// TestPrinter prints messages during testing
type TestPrinter struct {
	t testing.TB
}

func NewTestPrinter(t testing.TB) *TestPrinter {
	return &TestPrinter{
		t: t,
	}
}

var _ Printer = (*TestPrinter)(nil)

func (p *TestPrinter) NewCounter(_ string) *Counter {
	return nil
}

func (p *TestPrinter) NewCounterTerminalOnly(_ string) *Counter {
	return nil
}

func (p *TestPrinter) E(msg string, args ...interface{}) {
	p.t.Logf("error: "+msg, args...)
}

func (p *TestPrinter) S(msg string, args ...interface{}) {
	p.t.Logf("stdout: "+msg, args...)
}

func (p *TestPrinter) P(msg string, args ...interface{}) {
	p.t.Logf("print: "+msg, args...)
}

func (p *TestPrinter) V(msg string, args ...interface{}) {
	p.t.Logf("verbose: "+msg, args...)
}

func (p *TestPrinter) VV(msg string, args ...interface{}) {
	p.t.Logf("verbose2: "+msg, args...)
}
