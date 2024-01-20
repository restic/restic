package progress

// A Printer can can return a new counter or print messages
// at different log levels.
// It must be safe to call its methods from concurrent goroutines.
type Printer interface {
	NewCounter(description string) *Counter

	E(msg string, args ...interface{})
	P(msg string, args ...interface{})
	V(msg string, args ...interface{})
	VV(msg string, args ...interface{})
}

// NoopPrinter discards all messages
type NoopPrinter struct{}

var _ Printer = (*NoopPrinter)(nil)

func (*NoopPrinter) NewCounter(description string) *Counter {
	return nil
}

func (*NoopPrinter) E(msg string, args ...interface{}) {}

func (*NoopPrinter) P(msg string, args ...interface{}) {}

func (*NoopPrinter) V(msg string, args ...interface{}) {}

func (*NoopPrinter) VV(msg string, args ...interface{}) {}
