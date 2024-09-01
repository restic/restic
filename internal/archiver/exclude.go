package archiver

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/fs"
)

// RejectByNameFunc is a function that takes a filename of a
// file that would be included in the backup. The function returns true if it
// should be excluded (rejected) from the backup.
type RejectByNameFunc func(path string) bool

// RejectFunc is a function that takes a filename and os.FileInfo of a
// file that would be included in the backup. The function returns true if it
// should be excluded (rejected) from the backup.
type RejectFunc func(path string, fi os.FileInfo, fs fs.FS) bool

func CombineRejectByNames(funcs []RejectByNameFunc) SelectByNameFunc {
	return func(item string) bool {
		for _, reject := range funcs {
			if reject(item) {
				return false
			}
		}
		return true
	}
}

func CombineRejects(funcs []RejectFunc) SelectFunc {
	return func(item string, fi os.FileInfo, fs fs.FS) bool {
		for _, reject := range funcs {
			if reject(item, fi, fs) {
				return false
			}
		}
		return true
	}
}

type rejectionCache struct {
	m   map[string]bool
	mtx sync.Mutex
}

func newRejectionCache() *rejectionCache {
	return &rejectionCache{m: make(map[string]bool)}
}

// Lock locks the mutex in rc.
func (rc *rejectionCache) Lock() {
	rc.mtx.Lock()
}

// Unlock unlocks the mutex in rc.
func (rc *rejectionCache) Unlock() {
	rc.mtx.Unlock()
}

// Get returns the last stored value for dir and a second boolean that
// indicates whether that value was actually written to the cache. It is the
// callers responsibility to call rc.Lock and rc.Unlock before using this
// method, otherwise data races may occur.
func (rc *rejectionCache) Get(dir string) (bool, bool) {
	v, ok := rc.m[dir]
	return v, ok
}

// Store stores a new value for dir.  It is the callers responsibility to call
// rc.Lock and rc.Unlock before using this method, otherwise data races may
// occur.
func (rc *rejectionCache) Store(dir string, rejected bool) {
	rc.m[dir] = rejected
}

// RejectIfPresent returns a RejectByNameFunc which itself returns whether a path
// should be excluded. The RejectByNameFunc considers a file to be excluded when
// it resides in a directory with an exclusion file, that is specified by
// excludeFileSpec in the form "filename[:content]". The returned error is
// non-nil if the filename component of excludeFileSpec is empty. If rc is
// non-nil, it is going to be used in the RejectByNameFunc to expedite the evaluation
// of a directory based on previous visits.
func RejectIfPresent(excludeFileSpec string, warnf func(msg string, args ...interface{})) (RejectFunc, error) {
	if excludeFileSpec == "" {
		return nil, errors.New("name for exclusion tagfile is empty")
	}
	colon := strings.Index(excludeFileSpec, ":")
	if colon == 0 {
		return nil, fmt.Errorf("no name for exclusion tagfile provided")
	}
	tf, tc := "", ""
	if colon > 0 {
		tf = excludeFileSpec[:colon]
		tc = excludeFileSpec[colon+1:]
	} else {
		tf = excludeFileSpec
	}
	debug.Log("using %q as exclusion tagfile", tf)
	rc := newRejectionCache()
	return func(filename string, _ os.FileInfo, fs fs.FS) bool {
		return isExcludedByFile(filename, tf, tc, rc, fs, warnf)
	}, nil
}

// isExcludedByFile interprets filename as a path and returns true if that file
// is in an excluded directory. A directory is identified as excluded if it contains a
// tagfile which bears the name specified in tagFilename and starts with
// header. If rc is non-nil, it is used to expedite the evaluation of a
// directory based on previous visits.
func isExcludedByFile(filename, tagFilename, header string, rc *rejectionCache, fs fs.FS, warnf func(msg string, args ...interface{})) bool {
	if tagFilename == "" {
		return false
	}

	if fs.Base(filename) == tagFilename {
		return false // do not exclude the tagfile itself
	}
	rc.Lock()
	defer rc.Unlock()

	dir := fs.Dir(filename)
	rejected, visited := rc.Get(dir)
	if visited {
		return rejected
	}
	rejected = isDirExcludedByFile(dir, tagFilename, header, fs, warnf)
	rc.Store(dir, rejected)
	return rejected
}

func isDirExcludedByFile(dir, tagFilename, header string, fs fs.FS, warnf func(msg string, args ...interface{})) bool {
	tf := fs.Join(dir, tagFilename)
	_, err := fs.Lstat(tf)
	if errors.Is(err, os.ErrNotExist) {
		return false
	}
	if err != nil {
		warnf("could not access exclusion tagfile: %v", err)
		return false
	}
	// when no signature is given, the mere presence of tf is enough reason
	// to exclude filename
	if len(header) == 0 {
		return true
	}
	// From this stage, errors mean tagFilename exists but it is malformed.
	// Warnings will be generated so that the user is informed that the
	// indented ignore-action is not performed.
	f, err := fs.OpenFile(tf, os.O_RDONLY, 0)
	if err != nil {
		warnf("could not open exclusion tagfile: %v", err)
		return false
	}
	defer func() {
		_ = f.Close()
	}()
	buf := make([]byte, len(header))
	_, err = io.ReadFull(f, buf)
	// EOF is handled with a dedicated message, otherwise the warning were too cryptic
	if err == io.EOF {
		warnf("invalid (too short) signature in exclusion tagfile %q\n", tf)
		return false
	}
	if err != nil {
		warnf("could not read signature from exclusion tagfile %q: %v\n", tf, err)
		return false
	}
	if !bytes.Equal(buf, []byte(header)) {
		warnf("invalid signature in exclusion tagfile %q\n", tf)
		return false
	}
	return true
}

// deviceMap is used to track allowed source devices for backup. This is used to
// check for crossing mount points during backup (for --one-file-system). It
// maps the name of a source path to its device ID.
type deviceMap map[string]uint64

// newDeviceMap creates a new device map from the list of source paths.
func newDeviceMap(allowedSourcePaths []string, fs fs.FS) (deviceMap, error) {
	deviceMap := make(map[string]uint64)

	for _, item := range allowedSourcePaths {
		item, err := fs.Abs(fs.Clean(item))
		if err != nil {
			return nil, err
		}

		fi, err := fs.Lstat(item)
		if err != nil {
			return nil, err
		}

		id, err := fs.DeviceID(fi)
		if err != nil {
			return nil, err
		}

		deviceMap[item] = id
	}

	if len(deviceMap) == 0 {
		return nil, errors.New("zero allowed devices")
	}

	return deviceMap, nil
}

// IsAllowed returns true if the path is located on an allowed device.
func (m deviceMap) IsAllowed(item string, deviceID uint64, fs fs.FS) (bool, error) {
	for dir := item; ; dir = fs.Dir(dir) {
		debug.Log("item %v, test dir %v", item, dir)

		// find a parent directory that is on an allowed device (otherwise
		// we would not traverse the directory at all)
		allowedID, ok := m[dir]
		if !ok {
			if dir == fs.Dir(dir) {
				// arrived at root, no allowed device found. this should not happen.
				break
			}
			continue
		}

		// if the item has a different device ID than the parent directory,
		// we crossed a file system boundary
		if allowedID != deviceID {
			debug.Log("item %v (dir %v) on disallowed device %d", item, dir, deviceID)
			return false, nil
		}

		// item is on allowed device, accept it
		debug.Log("item %v allowed", item)
		return true, nil
	}

	return false, fmt.Errorf("item %v (device ID %v) not found, deviceMap: %v", item, deviceID, m)
}

// RejectByDevice returns a RejectFunc that rejects files which are on a
// different file systems than the files/dirs in samples.
func RejectByDevice(samples []string, filesystem fs.FS) (RejectFunc, error) {
	deviceMap, err := newDeviceMap(samples, filesystem)
	if err != nil {
		return nil, err
	}
	debug.Log("allowed devices: %v\n", deviceMap)

	return func(item string, fi os.FileInfo, fs fs.FS) bool {
		id, err := fs.DeviceID(fi)
		if err != nil {
			// This should never happen because gatherDevices() would have
			// errored out earlier. If it still does that's a reason to panic.
			panic(err)
		}

		allowed, err := deviceMap.IsAllowed(fs.Clean(item), id, fs)
		if err != nil {
			// this should not happen
			panic(fmt.Sprintf("error checking device ID of %v: %v", item, err))
		}

		if allowed {
			// accept item
			return false
		}

		// reject everything except directories
		if !fi.IsDir() {
			return true
		}

		// special case: make sure we keep mountpoints (directories which
		// contain a mounted file system). Test this by checking if the parent
		// directory would be included.
		parentDir := fs.Dir(fs.Clean(item))

		parentFI, err := fs.Lstat(parentDir)
		if err != nil {
			debug.Log("item %v: error running lstat() on parent directory: %v", item, err)
			// if in doubt, reject
			return true
		}

		parentDeviceID, err := fs.DeviceID(parentFI)
		if err != nil {
			debug.Log("item %v: getting device ID of parent directory: %v", item, err)
			// if in doubt, reject
			return true
		}

		parentAllowed, err := deviceMap.IsAllowed(parentDir, parentDeviceID, fs)
		if err != nil {
			debug.Log("item %v: error checking parent directory: %v", item, err)
			// if in doubt, reject
			return true
		}

		if parentAllowed {
			// we found a mount point, so accept the directory
			return false
		}

		// reject everything else
		return true
	}, nil
}

func RejectBySize(maxSize int64) (RejectFunc, error) {
	return func(item string, fi os.FileInfo, _ fs.FS) bool {
		// directory will be ignored
		if fi.IsDir() {
			return false
		}

		filesize := fi.Size()
		if filesize > maxSize {
			debug.Log("file %s is oversize: %d", item, filesize)
			return true
		}

		return false
	}, nil
}
