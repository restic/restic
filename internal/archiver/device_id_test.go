package archiver

import (
	"hash/fnv"
	"testing"

	rtest "github.com/restic/restic/internal/test"
)

// pathDeviceID returns the device ID derived from snPath.
func pathDeviceID(t testing.TB, snPath string) uint64 {
	t.Helper()

	h := fnv.New64a()
	_, err := h.Write([]byte(snPath))
	rtest.OK(t, err)
	return h.Sum64()
}

func TestDeviceIDMapDerivedFromPath(t *testing.T) {
	m := deviceIDMap{}

	dev := m.resolve("/mnt/data/file", 0, 42)
	rtest.Equals(t, pathDeviceID(t, "/mnt/data/file"), dev)

	// the ID of a device is only derived from the first node found on it
	rtest.Equals(t, dev, m.resolve("/mnt/data/other", 0, 42))
	rtest.Equals(t, deviceIDMap{42: dev}, m)
}

func TestDeviceIDMapFromParentSnapshot(t *testing.T) {
	m := deviceIDMap{}

	// the device ID from the parent snapshot must be reused even though the
	// filesystem now reports a different one
	dev := m.resolve("/mnt/data/file", 1234, 42)
	rtest.Equals(t, uint64(1234), dev)
	rtest.Equals(t, dev, m.resolve("/mnt/data/other", 5678, 42))
}

func TestDeviceIDMapIgnoresEmptyParentDeviceID(t *testing.T) {
	m := deviceIDMap{}

	// a parent node without a device ID must not poison the map
	dev := m.resolve("/mnt/data/file", 0, 42)
	rtest.Equals(t, pathDeviceID(t, "/mnt/data/file"), dev)
}

func TestDeviceIDMapCollision(t *testing.T) {
	// pretend that the ID derived from the path is already used by another device
	collision := pathDeviceID(t, "/mnt/data/file")
	m := deviceIDMap{23: collision}

	dev := m.resolve("/mnt/data/file", 0, 42)
	rtest.Assert(t, dev != collision, "device ID %v is used twice", dev)
	rtest.Assert(t, dev != 0, "device ID must not be zero")
}

func TestDeviceIDMapParentSnapshotCollision(t *testing.T) {
	m := deviceIDMap{}

	// Two devices that were a single one in the parent snapshot must not share
	// a device ID. That would create bogus hardlinks on restore.
	dev := m.resolve("/mnt/data/file", 1234, 42)
	rtest.Equals(t, uint64(1234), dev)

	other := m.resolve("/mnt/other/file", 1234, 23)
	rtest.Equals(t, pathDeviceID(t, "/mnt/other/file"), other)
}

func TestDeviceIDMapIndependentOfOrder(t *testing.T) {
	first := deviceIDMap{}
	devData := first.resolve("/mnt/data/file", 0, 42)
	devHome := first.resolve("/mnt/home/file", 0, 23)

	// The second backup run mounts a new device and visits it before the
	// already known ones. Their device IDs must not change.
	second := deviceIDMap{}
	devBackup := second.resolve("/mnt/backup/file", 0, 5)
	rtest.Equals(t, devData, second.resolve("/mnt/data/file", 0, 42))
	rtest.Equals(t, devHome, second.resolve("/mnt/home/file", 0, 23))

	rtest.Assert(t, devBackup != devData && devBackup != devHome,
		"device IDs must be unique, got %v for the new device", devBackup)
}

func TestDeviceIDMapNewFirstNodeWithoutParent(t *testing.T) {
	// First backup run. The device is first seen at /mnt/data/zzz.
	first := deviceIDMap{}
	dev := first.resolve("/mnt/data/zzz", 0, 42)

	// In the second run a new hardlinked file sorts before it and has no device
	// ID to inherit. The whole device is renumbered from the new path once and
	// the next backup run inherits the new ID again.
	second := deviceIDMap{}
	rtest.Equals(t, pathDeviceID(t, "/mnt/data/aaa"), second.resolve("/mnt/data/aaa", 0, 42))
	rtest.Equals(t, pathDeviceID(t, "/mnt/data/aaa"), second.resolve("/mnt/data/zzz", dev, 42))
}

func TestDeviceIDMapNeverZero(t *testing.T) {
	m := deviceIDMap{}
	for _, snPath := range []string{"/", "", "/mnt/data/file"} {
		rtest.Assert(t, m.generate(snPath) != 0, "device ID for %q must not be zero", snPath)
	}
}
