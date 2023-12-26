package restorer_test

import (
	"testing"

	"github.com/restic/restic/internal/restorer"
	rtest "github.com/restic/restic/internal/test"
)

// TestHardLinks contains various tests for HardlinkIndex.
func TestHardLinks(t *testing.T) {

	idx := restorer.NewHardlinkIndex[string]()

	idx.Add(1, 2, "inode1-file1-on-device2")
	idx.Add(2, 3, "inode2-file2-on-device3")

	sresult := idx.Value(1, 2)
	rtest.Equals(t, sresult, "inode1-file1-on-device2")

	sresult = idx.Value(2, 3)
	rtest.Equals(t, sresult, "inode2-file2-on-device3")

	bresult := idx.Has(1, 2)
	rtest.Equals(t, bresult, true)

	bresult = idx.Has(1, 3)
	rtest.Equals(t, bresult, false)

	idx.Remove(1, 2)
	bresult = idx.Has(1, 2)
	rtest.Equals(t, bresult, false)
}
