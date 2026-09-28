//go:build linux

package repository

import "golang.org/x/sys/unix"

// openTreeGroupsBudget derives the grouped-repack budget from the soft
// file-descriptor limit (RLIMIT_NOFILE), reserving some descriptors for other
// open handles. The result is clamped by the caller (MaxOpenTreeGroups).
func openTreeGroupsBudget() int {
	var rlim unix.Rlimit
	if err := unix.Getrlimit(unix.RLIMIT_NOFILE, &rlim); err != nil {
		return defaultOpenTreeGroups
	}
	// rlim.Cur is the soft limit (uint64 on Linux). Guard against very large or
	// "infinity" values before narrowing to int; the clamp caps it anyway.
	if rlim.Cur > uint64(maxOpenTreeGroupsCap+openFDReserve) {
		return maxOpenTreeGroupsCap
	}
	return int(rlim.Cur) - openFDReserve
}
