//go:build !windows
// +build !windows

package restorer

// toComparableFilename returns a filename suitable for equality checks. On Windows, it returns the
// uppercase version of the string. On all other systems, it returns the unmodified filename.
func toComparableFilename(path string) string {
	return path
}
