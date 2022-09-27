//go:build !go1.19
// +build !go1.19

// This file provides a stub for IsErrDot() for Go versions below 1.19.
// See the corresponding file errdot_119.go for more information.
// Once the minimum Go version restic supports is 1.19, remove this file
//  and perform the actions listed in errdot_119.go.

package backend

func IsErrDot(err error) bool {
	return false
}
