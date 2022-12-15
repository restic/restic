package errors

import (
	"errors"
	"fmt"
)

// fatalError is an error that should be printed to the user, then the program
// should exit with an error code.
type fatalError string

func (e fatalError) Error() string {
	return string(e)
}

// IsFatal returns true if err is a fatal message that should be printed to the
// user. Then, the program should exit.
func IsFatal(err error) bool {
	var fatal fatalError
	return errors.As(err, &fatal)
}

// Fatal returns an error that is marked fatal.
func Fatal(s string) error {
	return Wrap(fatalError(s), "Fatal")
}

// Fatalf returns an error that is marked fatal.
func Fatalf(s string, data ...interface{}) error {
	return Wrap(fatalError(fmt.Sprintf(s, data...)), "Fatal")
}
