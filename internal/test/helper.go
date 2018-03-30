// +build go1.9

package test

import "testing"

// Helperer marks the current function as a test helper.
type Helperer interface {
	Helper()
}

// Helper returns a function that marks the current function as a helper function.
func Helper(t testing.TB) Helperer {
	return t
}
