package fs

import (
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/restic/restic/internal/errors"
)

// ErrorHandler is used to report errors via callback
type ErrorHandler func(item string, err error) error

// MessageHandler is used to report errors/messages via callbacks.
type MessageHandler func(msg string, args ...interface{})

// LocalVss is a wrapper around the local file system which uses windows volume
// shadow copy service (VSS) in a transparent way.
type LocalVss struct {
	FS
	snapshots       map[string]VssSnapshot
	failedSnapshots map[string]struct{}
	mutex           sync.RWMutex
	msgError        ErrorHandler
	msgMessage      MessageHandler
}

// statically ensure that LocalVss implements FS.
var _ FS = &LocalVss{}

// NewLocalVss creates a new wrapper around the windows filesystem using volume
// shadow copy service to access locked files.
func NewLocalVss(msgError ErrorHandler, msgMessage MessageHandler) *LocalVss {
	return &LocalVss{
		FS:              Local{},
		snapshots:       make(map[string]VssSnapshot),
		failedSnapshots: make(map[string]struct{}),
		msgError:        msgError,
		msgMessage:      msgMessage,
	}
}

// DeleteSnapshots deletes all snapshots that were created automatically.
func (fs *LocalVss) DeleteSnapshots() {
	fs.mutex.Lock()
	defer fs.mutex.Unlock()

	activeSnapshots := make(map[string]VssSnapshot)

	for volumeName, snapshot := range fs.snapshots {
		if err := snapshot.Delete(); err != nil {
			_ = fs.msgError(volumeName, errors.Errorf("failed to delete VSS snapshot: %s", err))
			activeSnapshots[volumeName] = snapshot
		}
	}

	fs.snapshots = activeSnapshots
}

// Open  wraps the Open method of the underlying file system.
func (fs *LocalVss) Open(name string) (File, error) {
	return os.Open(fs.snapshotPath(name))
}

// OpenFile wraps the Open method of the underlying file system.
func (fs *LocalVss) OpenFile(name string, flag int, perm os.FileMode) (File, error) {
	return os.OpenFile(fs.snapshotPath(name), flag, perm)
}

// Stat wraps the Open method of the underlying file system.
func (fs *LocalVss) Stat(name string) (os.FileInfo, error) {
	return os.Stat(fs.snapshotPath(name))
}

// Lstat wraps the Open method of the underlying file system.
func (fs *LocalVss) Lstat(name string) (os.FileInfo, error) {
	return os.Lstat(fs.snapshotPath(name))
}

// snapshotPath returns the path inside a VSS snapshots if it already exists.
// If the path is not yet available as a snapshot, a snapshot is created.
// If creation of a snapshot fails the file's original path is returned as
// a fallback.
func (fs *LocalVss) snapshotPath(path string) string {

	fixPath := fixpath(path)

	if strings.HasPrefix(fixPath, `\\?\UNC\`) {
		// UNC network shares are currently not supported so we access the regular file
		// without snapshotting
		// TODO: right now there is a problem in fixpath(): "\\host\share" is not returned as a UNC path
		//       "\\host\share\" is returned as a valid UNC path
		return path
	}

	fixPath = strings.TrimPrefix(fixpath(path), `\\?\`)
	fixPathLower := strings.ToLower(fixPath)
	volumeName := filepath.VolumeName(fixPath)
	volumeNameLower := strings.ToLower(volumeName)

	fs.mutex.RLock()

	// ensure snapshot for volume exists
	_, snapshotExists := fs.snapshots[volumeNameLower]
	_, snapshotFailed := fs.failedSnapshots[volumeNameLower]
	if !snapshotExists && !snapshotFailed {
		fs.mutex.RUnlock()
		fs.mutex.Lock()
		defer fs.mutex.Unlock()

		_, snapshotExists = fs.snapshots[volumeNameLower]
		_, snapshotFailed = fs.failedSnapshots[volumeNameLower]

		if !snapshotExists && !snapshotFailed {
			vssVolume := volumeNameLower + string(filepath.Separator)
			fs.msgMessage("creating VSS snapshot for [%s]\n", vssVolume)

			if snapshot, err := NewVssSnapshot(vssVolume, 120, fs.msgError); err != nil {
				_ = fs.msgError(vssVolume, errors.Errorf("failed to create snapshot for [%s]: %s",
					vssVolume, err))
				fs.failedSnapshots[volumeNameLower] = struct{}{}
			} else {
				fs.snapshots[volumeNameLower] = snapshot
				fs.msgMessage("successfully created snapshot for [%s]\n", vssVolume)
				if len(snapshot.mountPointInfo) > 0 {
					fs.msgMessage("mountpoints in snapshot volume [%s]:\n", vssVolume)
					for mp, mpInfo := range snapshot.mountPointInfo {
						info := ""
						if !mpInfo.IsSnapshotted() {
							info = " (not snapshotted)"
						}
						fs.msgMessage(" - %s%s\n", mp, info)
					}
				}
			}
		}
	} else {
		defer fs.mutex.RUnlock()
	}

	var snapshotPath string
	if snapshot, ok := fs.snapshots[volumeNameLower]; ok {
		// handle case when data is inside mountpoint
		for mountPoint, info := range snapshot.mountPointInfo {
			if HasPathPrefix(mountPoint, fixPathLower) {
				if !info.IsSnapshotted() {
					// requested path is under mount point but mount point is
					// not available as a snapshot (e.g. no filesystem support,
					// removable media, etc.)
					//  -> try to backup without a snapshot
					return path
				}

				// filepath.rel() should always succeed because we checked that fixPath is either
				// the same path or below mountPoint and operation is case-insensitive
				relativeToMount, err := filepath.Rel(mountPoint, fixPath)
				if err != nil {
					panic(err)
				}

				snapshotPath = fs.Join(info.GetSnapshotDeviceObject(), relativeToMount)

				if snapshotPath == info.GetSnapshotDeviceObject() {
					snapshotPath += string(filepath.Separator)
				}

				return snapshotPath
			}
		}

		// requested data is directly on the volume, not inside a mount point
		snapshotPath = fs.Join(snapshot.GetSnapshotDeviceObject(),
			strings.TrimPrefix(fixPath, volumeName))
		if snapshotPath == snapshot.GetSnapshotDeviceObject() {
			snapshotPath = snapshotPath + string(filepath.Separator)
		}

	} else {
		// no snapshot is available for the requested path:
		//  -> try to backup without a snapshot
		// TODO: log warning?
		snapshotPath = path
	}

	return snapshotPath
}
