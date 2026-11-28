//go:build !linux

package archiver

// isNoDump returns whether the "no dump" Linux file attribute is set on path.
// See CHATTR(1) for more information about Linux file attributes.
func isNoDump(path string) (bool, error) {
	return false, nil
}
