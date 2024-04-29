//go:build !windows
// +build !windows

package fs

import (
	"time"

	"github.com/restic/restic/internal/errors"
)

// MountPoint is a dummy for non-windows platforms to let client code compile.
type MountPoint struct {
}

// IsSnapshotted is true if this mount point was snapshotted successfully.
func (p *MountPoint) IsSnapshotted() bool {
	return false
}

// GetSnapshotDeviceObject returns root path to access the snapshot files and folders.
func (p *MountPoint) GetSnapshotDeviceObject() string {
	return ""
}

// VssSnapshot is a dummy for non-windows platforms to let client code compile.
type VssSnapshot struct {
	mountPointInfo map[string]MountPoint
}

// HasSufficientPrivilegesForVSS returns true if the user is allowed to use VSS.
func HasSufficientPrivilegesForVSS() error {
	return errors.New("VSS snapshots are only supported on windows")
}

// GetVolumeNameForVolumeMountPoint add trailing backslash to input parameter
// and calls the equivalent windows api.
func GetVolumeNameForVolumeMountPoint(mountPoint string) (string, error) {
	return mountPoint, nil
}

// NewVssSnapshot creates a new vss snapshot. If creating the snapshots doesn't
// finish within the timeout an error is returned.
func NewVssSnapshot(_ string,
	_ string, _ time.Duration, _ VolumeFilter, _ ErrorHandler) (VssSnapshot, error) {
	return VssSnapshot{}, errors.New("VSS snapshots are only supported on windows")
}

// Delete deletes the created snapshot.
func (p *VssSnapshot) Delete() error {
	return nil
}

// GetSnapshotDeviceObject returns root path to access the snapshot files
// and folders.
func (p *VssSnapshot) GetSnapshotDeviceObject() string {
	return ""
}
