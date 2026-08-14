package archiver

import (
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
