package errors

import (
	stderrors "errors"

	"github.com/pkg/errors"
)

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

// WithStack annotates err with a stack trace at the point WithStack was called.
// If err is nil, WithStack returns nil.
var WithStack = errors.WithStack

// Go 1.13-style error handling.

// As finds the first error in err's tree that matches target, and if one is found,
// sets target to that error value and returns true. Otherwise, it returns false.
func As(err error, tgt interface{}) bool { return stderrors.As(err, tgt) }

// Is reports whether any error in err's tree matches target.
func Is(x, y error) bool { return stderrors.Is(x, y) }

func Join(errs ...error) error { return stderrors.Join(errs...) }

// Unwrap returns the result of calling the Unwrap method on err, if err's type contains
// an Unwrap method returning error. Otherwise, Unwrap returns nil.
//
// Unwrap only calls a method of the form "Unwrap() error". In particular Unwrap does not
// unwrap errors returned by [Join].
func Unwrap(err error) error { return stderrors.Unwrap(err) }
