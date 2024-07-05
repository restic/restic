//go:build windows
// +build windows

package restorer

import "strings"

// toComparableFilename returns a filename suitable for equality checks. On Windows, it returns the
// uppercase version of the string. On all other systems, it returns the unmodified filename.
func toComparableFilename(path string) string {
	// apparently NTFS internally uppercases filenames for comparision
	return strings.ToUpper(path)
}
