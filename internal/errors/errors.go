package errors

import (
	stderrors "errors"
	"net/url"

	"github.com/cenkalti/backoff"
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

var WithStack = errors.WithStack

// Cause returns the cause of an error. It will also unwrap certain errors,
// e.g. *url.Error returned by the net/http client.
func Cause(err error) error {
	type Causer interface {
		Cause() error
	}

	for {
		switch e := err.(type) {
		case Causer: // github.com/pkg/errors
			err = e.Cause()
		case *backoff.PermanentError:
			err = e.Err
		case *url.Error:
			err = e.Err
		default:
			return err
		}
	}
}

// Go 1.13-style error handling.

func As(err error, tgt interface{}) bool { return stderrors.As(err, tgt) }

func Is(x, y error) bool { return stderrors.Is(x, y) }

func Unwrap(err error) error { return stderrors.Unwrap(err) }
