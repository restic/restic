//go:build !unix

package fileio

// deviceID has no counterpart outside unix; callers fall back to the volume name.
func deviceID(_ string) (uint64, bool) {
	return 0, false
}
