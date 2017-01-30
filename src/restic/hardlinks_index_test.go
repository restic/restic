package restic_test

import (
	"testing"

	"restic"
	. "restic/test"
)

// TestHardLinks contains various tests for the HardlinkIndex
func TestHardLinks(t *testing.T) {

	idx := restic.NewHardlinkIndex()

	idx.AddLink(1, 2, "inode1-file1-on-device2")
	idx.AddLink(2, 3, "inode2-file2-on-device3")

	var sresult string
	sresult = idx.GetLinkName(1, 2)
	Assert(t, sresult == "inode1-file1-on-device2",
		"Name doesn't match (%v != %v)", sresult, "inode1-file1-on-device2")

	sresult = idx.GetLinkName(2, 3)
	Assert(t, sresult == "inode2-file2-on-device3",
		"Name doesn't match (%v != %v)", sresult, "inode2-file2-on-device3")

	var bresult bool
	bresult = idx.ExistsLink(1, 2)
	Assert(t, bresult == true,
		"Existence does not match (%v != %v)", bresult, true)

	bresult = idx.ExistsLink(1, 3)
	Assert(t, bresult == false,
		"Existence does not match (%v != %v)", bresult, false)

	idx.RemoveLink(1, 2)
	bresult = idx.ExistsLink(1, 2)
	Assert(t, bresult == false,
		"Existence does not match (%v != %v)", bresult, false)
}
