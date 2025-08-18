package errors

import (
	"errors"
	"fmt"
)

// fatalError is an error that should be printed to the user, then the program
// should exit with an error code.
type fatalError struct {
	msg string
	err error // Underlying error
}

func (e *fatalError) Error() string {
	return e.msg
}

func (e *fatalError) Unwrap() error {
	return e.err
}

// IsFatal returns true if err is a fatal message that should be printed to the
// user. Then, the program should exit.
func IsFatal(err error) bool {
	var fatal *fatalError
	return errors.As(err, &fatal)
}

// Fatal returns an error that is marked fatal.
func Fatal(s string) error {
	return Wrap(&fatalError{msg: s}, "Fatal")
}

// Fatalf returns an error that is marked fatal, preserving an underlying error if passed.
func Fatalf(s string, data ...interface{}) error {
	// Use the last error found.
	var underlyingErr error
	for i := len(data) - 1; i >= 0; i-- {
		if err, ok := data[i].(error); ok {
			underlyingErr = err
			break
		}
	}

	msg := fmt.Sprintf(s, data...)

	fatal := &fatalError{
		msg: msg,
		err: underlyingErr,
	}

	return Wrap(fatal, "Fatal")
}
