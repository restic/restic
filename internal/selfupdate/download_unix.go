//go:build !windows

package selfupdate

// Remove the target binary.
func removeResticBinary(_, _ string) error {
	// removed on rename on this platform
	return nil
}
