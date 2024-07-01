package fs

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/options"
)

// VSSConfig holds extended options of windows volume shadow copy service.
type VSSConfig struct {
	ExcludeAllMountPoints bool          `option:"exclude-all-mount-points" help:"exclude mountpoints from snapshotting on all volumes"`
	ExcludeVolumes        string        `option:"exclude-volumes" help:"semicolon separated list of volumes to exclude from snapshotting (ex. 'c:\\;e:\\mnt;\\\\?\\Volume{...}')"`
	Timeout               time.Duration `option:"timeout" help:"time that the VSS can spend creating snapshot before timing out"`
	Provider              string        `option:"provider" help:"VSS provider identifier which will be used for snapshotting"`
}

func init() {
	if runtime.GOOS == "windows" {
		options.Register("vss", VSSConfig{})
	}
}

// NewVSSConfig returns a new VSSConfig with the default values filled in.
func NewVSSConfig() VSSConfig {
	return VSSConfig{
		Timeout: time.Second * 120,
	}
}

// ParseVSSConfig parses a VSS extended options to VSSConfig struct.
func ParseVSSConfig(o options.Options) (VSSConfig, error) {
	cfg := NewVSSConfig()
	o = o.Extract("vss")
	if err := o.Apply("vss", &cfg); err != nil {
		return VSSConfig{}, err
	}

	return cfg, nil
}

// ErrorHandler is used to report errors via callback.
type ErrorHandler func(item string, err error)

// MessageHandler is used to report errors/messages via callbacks.
type MessageHandler func(msg string, args ...interface{})

// VolumeFilter is used to filter volumes by it's mount point or GUID path.
type VolumeFilter func(volume string) bool

// LocalVss is a wrapper around the local file system which uses windows volume
// shadow copy service (VSS) in a transparent way.
type LocalVss struct {
	FS
	snapshots             map[string]VssSnapshot
	failedSnapshots       map[string]struct{}
	mutex                 sync.RWMutex
	msgError              ErrorHandler
	msgMessage            MessageHandler
	excludeAllMountPoints bool
	excludeVolumes        map[string]struct{}
	timeout               time.Duration
	provider              string
}

// statically ensure that LocalVss implements FS.
var _ FS = &LocalVss{}

// parseMountPoints try to convert semicolon separated list of mount points
// to map of lowercased volume GUID paths. Mountpoints already in volume
// GUID path format will be validated and normalized.
func parseMountPoints(list string, msgError ErrorHandler) (volumes map[string]struct{}) {
	if list == "" {
		return
	}
	for _, s := range strings.Split(list, ";") {
		if v, err := GetVolumeNameForVolumeMountPoint(s); err != nil {
			msgError(s, errors.Errorf("failed to parse vss.exclude-volumes [%s]: %s", s, err))
		} else {
			if volumes == nil {
				volumes = make(map[string]struct{})
			}
			volumes[strings.ToLower(v)] = struct{}{}
		}
	}

	return
}

// NewLocalVss creates a new wrapper around the windows filesystem using volume
// shadow copy service to access locked files.
func NewLocalVss(msgError ErrorHandler, msgMessage MessageHandler, cfg VSSConfig) *LocalVss {
	return &LocalVss{
		FS:                    Local{},
		snapshots:             make(map[string]VssSnapshot),
		failedSnapshots:       make(map[string]struct{}),
		msgError:              msgError,
		msgMessage:            msgMessage,
		excludeAllMountPoints: cfg.ExcludeAllMountPoints,
		excludeVolumes:        parseMountPoints(cfg.ExcludeVolumes, msgError),
		timeout:               cfg.Timeout,
		provider:              cfg.Provider,
	}
}

// DeleteSnapshots deletes all snapshots that were created automatically.
func (fs *LocalVss) DeleteSnapshots() {
	fs.mutex.Lock()
	defer fs.mutex.Unlock()

	activeSnapshots := make(map[string]VssSnapshot)

	for volumeName, snapshot := range fs.snapshots {
		if err := snapshot.Delete(); err != nil {
			fs.msgError(volumeName, errors.Errorf("failed to delete VSS snapshot: %s", err))
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

// isMountPointIncluded  is true if given mountpoint included by user.
func (fs *LocalVss) isMountPointIncluded(mountPoint string) bool {
	if fs.excludeVolumes == nil {
		return true
	}

	volume, err := GetVolumeNameForVolumeMountPoint(mountPoint)
	if err != nil {
		fs.msgError(mountPoint, errors.Errorf("failed to get volume from mount point [%s]: %s", mountPoint, err))
		return true
	}

	_, ok := fs.excludeVolumes[strings.ToLower(volume)]
	return !ok
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

			if !fs.isMountPointIncluded(vssVolume) {
				fs.msgMessage("snapshots for [%s] excluded by user\n", vssVolume)
				fs.failedSnapshots[volumeNameLower] = struct{}{}
			} else {
				fs.msgMessage("creating VSS snapshot for [%s]\n", vssVolume)

				var includeVolume VolumeFilter
				if !fs.excludeAllMountPoints {
					includeVolume = func(volume string) bool {
						return fs.isMountPointIncluded(volume)
					}
				}

				if snapshot, err := NewVssSnapshot(fs.provider, vssVolume, fs.timeout, includeVolume, fs.msgError); err != nil {
					fs.msgError(vssVolume, errors.Errorf("failed to create snapshot for [%s]: %s",
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
			snapshotPath += string(filepath.Separator)
		}
	} else {
		// no snapshot is available for the requested path:
		//  -> try to backup without a snapshot
		// TODO: log warning?
		snapshotPath = path
	}

	return snapshotPath
}
