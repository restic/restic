// +build !debug,!profile

package restic

// runDebug is a noop without the debug tag.
func runDebug() error { return nil }
