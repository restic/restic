//go:build !windows
// +build !windows

package selfupdate

// Remove the target binary.
func removeResticBinary(dir, target string) error {
	// removed on rename on this platform
	return nil
}
