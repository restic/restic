package terminal

// IsProcessBackground reports whether the current process is running in the
// background. Not implemented for this platform.
func IsProcessBackground(_ uintptr) bool {
	return false
}
