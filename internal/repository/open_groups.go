package repository

// These constants govern how many snapshot groups a grouped prune
// (`prune --group-by`) keeps open simultaneously. Each open group holds one
// temporary pack file, so the budget is derived from the process
// file-descriptor limit rather than a dedicated command-line option: operators
// scale it with `ulimit -n`.
const (
	// openFDReserve is the number of file descriptors kept aside for backend
	// connections, index files and other open handles when deriving the budget
	// from the file-descriptor limit.
	openFDReserve = 128
	// minOpenTreeGroups and maxOpenTreeGroupsCap clamp the derived budget. The
	// upper bound also limits worst-case temporary disk usage during a grouped
	// repack (roughly maxOpenTreeGroupsCap * packSize).
	minOpenTreeGroups    = 32
	maxOpenTreeGroupsCap = 2048
	// defaultOpenTreeGroups is used when the file-descriptor limit cannot be
	// determined (non-Linux platforms or a failed query).
	defaultOpenTreeGroups = 256
)

// MaxOpenTreeGroups returns how many snapshot groups may have an open pack file
// at the same time during a grouped prune, and equivalently how many groups
// prune will localize. Groups beyond this budget share a common pack (they lose
// locality but are still packed normally). The value is derived from the process
// file-descriptor limit so it scales with `ulimit -n` without a dedicated
// option.
func MaxOpenTreeGroups() int {
	return clampOpenTreeGroups(openTreeGroupsBudget())
}

func clampOpenTreeGroups(n int) int {
	if n < minOpenTreeGroups {
		return minOpenTreeGroups
	}
	if n > maxOpenTreeGroupsCap {
		return maxOpenTreeGroupsCap
	}
	return n
}
