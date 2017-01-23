// +build !debug

package main

// runDebug is a noop without the debug tag.
func runDebug() error { return nil }

// shutdownDebug is a noop without the debug tag.
func shutdownDebug() {}
