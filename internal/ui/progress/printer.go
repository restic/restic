package progress

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
	// PT prints a message if verbosity >= 1  (neither --quiet nor --verbose is specified)
	// and stdout points to a terminal.
	// This is used for informational messages.
	PT(msg string, args ...interface{})
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

// noopPrinter discards all messages.
type noopPrinter struct{}

var _ Printer = (*noopPrinter)(nil)

// NewNoopPrinter returns a Printer that discards all messages.
func NewNoopPrinter() Printer {
	return &noopPrinter{}
}

func (*noopPrinter) NewCounter(_ string) *Counter {
	return nil
}

func (*noopPrinter) NewCounterTerminalOnly(_ string) *Counter {
	return nil
}

func (*noopPrinter) E(_ string, _ ...interface{}) {}

func (*noopPrinter) S(_ string, _ ...interface{}) {}

func (*noopPrinter) PT(_ string, _ ...interface{}) {}

func (*noopPrinter) P(_ string, _ ...interface{}) {}

func (*noopPrinter) V(_ string, _ ...interface{}) {}

func (*noopPrinter) VV(_ string, _ ...interface{}) {}
