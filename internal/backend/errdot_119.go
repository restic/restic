//go:build go1.19
// +build go1.19

// This file provides a function to check whether an error from cmd.Start() is
//  exec.ErrDot which was introduced in Go 1.19.
// This function is needed so that we can perform this check only for Go 1.19 and
//  up, whereas for older versions we use a dummy/stub in the file errdot_old.go.
// Once the minimum Go version restic supports is 1.19, remove this file and
//  replace any calls to it with the corresponding code as per below.

package backend

import (
	"errors"
	"os/exec"
)

func IsErrDot(err error) bool {
	return errors.Is(err, exec.ErrDot)
}
