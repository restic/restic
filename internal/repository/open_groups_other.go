//go:build !linux

package repository

// openTreeGroupsBudget returns a fixed default on platforms where the
// file-descriptor limit is not queried. The grouped-repack feature is primarily
// targeted at Linux backup servers; extending this to other unix systems is a
// matter of adding a Getrlimit call analogous to the Linux implementation.
func openTreeGroupsBudget() int {
	return defaultOpenTreeGroups
}
