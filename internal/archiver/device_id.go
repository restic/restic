package archiver

import (
	"hash/fnv"
	"math/rand/v2"

	"github.com/restic/restic/internal/data"
	"github.com/restic/restic/internal/feature"
)

// storesDeviceID reports whether a node stores the ID of the device it is
// located on. Only hardlinked nodes need it on restore. See deviceIDMap.
//
// links must be the link count as stored in the node. Types that cannot be
// hardlinked leave it at zero.
func storesDeviceID(nodeType data.NodeType, links uint64) bool {
	if !feature.Flag.Enabled(feature.DeviceIDForHardlinks) {
		return true
	}
	return links > 1 && nodeType != data.NodeTypeDir
}

// deviceIDMap maps the device IDs reported by the filesystem to the virtual
// device IDs stored in a snapshot.
//
// The stored value only has to be shared by all files on a device within a
// snapshot so that the restorer can recreate hardlinks. btrfs and ZFS report a
// new device ID every time a subvolume or a filesystem snapshot is mounted.
// That modifies the metadata of every hardlinked file and makes restic upload
// new tree blobs. A virtual device ID stays the same across backup runs.
//
// Must only be used from a single goroutine. That avoids locking and keeps the
// result deterministic.
type deviceIDMap map[uint64]uint64

// resolve returns the virtual device ID for realDev. snPath is the path of the
// node within the snapshot. previousDev is the device ID that the parent
// snapshot stored for the same node and zero if there is none.
//
// The first node found on a device decides the ID for the whole device. The ID
// from the parent snapshot is reused unless another device already took it.
// Reusing an ID would create bogus hardlinks on restore. Otherwise the ID is
// derived from snPath and stays the same no matter in what order the devices
// are visited.
func (m deviceIDMap) resolve(snPath string, previousDev uint64, realDev uint64) uint64 {
	if dev, ok := m[realDev]; ok {
		return dev
	}

	var dev uint64
	if previousDev != 0 && !m.used(previousDev) {
		dev = previousDev
	} else {
		dev = m.generate(snPath)
	}

	m[realDev] = dev
	return dev
}

// generate returns a virtual device ID that is not used by the map yet.
func (m deviceIDMap) generate(snPath string) uint64 {
	// The hash must not change between restic runs. hash/maphash is seeded
	// randomly for each process and cannot be used here.
	h := fnv.New64a()
	_, _ = h.Write([]byte(snPath))
	dev := h.Sum64()

	// zero marks a node without device ID and must not be used
	for dev == 0 || m.used(dev) {
		dev = rand.Uint64()
	}
	return dev
}

// used reports whether dev is already assigned to some device.
func (m deviceIDMap) used(dev uint64) bool {
	for _, mapped := range m {
		if mapped == dev {
			return true
		}
	}
	return false
}
