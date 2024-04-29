//go:build !debug && !profile
// +build !debug,!profile

package main

// runDebug is a noop without the debug tag.
func runDebug() error { return nil }

// stopDebug is a noop without the debug tag.
func stopDebug() {}
