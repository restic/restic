package errors

import "github.com/pkg/errors"

// Cause returns the cause of an error.
func Cause(err error) error {
	return errors.Cause(err)
}

// New creates a new error based on message. Wrapped so that this package does
// not appear in the stack trace.
var New = errors.New

// Errorf creates an error based on a format string and values. Wrapped so that
// this package does not appear in the stack trace.
var Errorf = errors.Errorf

// Wrap wraps an error retrieved from outside of restic. Wrapped so that this
// package does not appear in the stack trace.
var Wrap = errors.Wrap

// Wrapf returns an error annotating err with the format specifier. If err is
// nil, Wrapf returns nil.
var Wrapf = errors.Wrapf
