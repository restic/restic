package errors

import "fmt"

// fatalError is an error that should be printed to the user, then the program
// should exit with an error code.
type fatalError string

func (e fatalError) Error() string {
	return string(e)
}

func (e fatalError) Fatal() bool {
	return true
}

// Fataler is an error which should be printed to the user directly.
// Afterwards, the program should exit with an error.
type Fataler interface {
	Fatal() bool
}

// IsFatal returns true if err is a fatal message that should be printed to the
// user. Then, the program should exit.
func IsFatal(err error) bool {
	e, ok := err.(Fataler)
	return ok && e.Fatal()
}

// Fatal returns a wrapped error which implements the Fataler interface.
func Fatal(s string) error {
	return Wrap(fatalError(s), "Fatal")
}

// Fatalf returns an error which implements the Fataler interface.
func Fatalf(s string, data ...interface{}) error {
	return fatalError(fmt.Sprintf(s, data...))
}
