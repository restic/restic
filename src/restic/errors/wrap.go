package errors

import "github.com/pkg/errors"

// Cause returns the cause of an error.
func Cause(err error) error {
	return errors.Cause(err)
}

// New creates a new error based on message.
func New(message string) error {
	return errors.New(message)
}

// Errorf creates an error based on a format string and values.
func Errorf(format string, args ...interface{}) error {
	return errors.Errorf(format, args...)
}

// Wrap wraps an error retrieved from outside of restic.
func Wrap(err error, message string) error {
	return errors.Wrap(err, message)
}
