package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/filter"
	"github.com/restic/restic/internal/fs"
	"github.com/restic/restic/internal/repository"
)

type rejectionCache struct {
	m   map[string]bool
	mtx sync.Mutex
}

// Lock locks the mutex in rc.
func (rc *rejectionCache) Lock() {
	if rc != nil {
		rc.mtx.Lock()
	}
}

// Unlock unlocks the mutex in rc.
func (rc *rejectionCache) Unlock() {
	if rc != nil {
		rc.mtx.Unlock()
	}
}

// Get returns the last stored value for dir and a second boolean that
// indicates whether that value was actually written to the cache. It is the
// callers responsibility to call rc.Lock and rc.Unlock before using this
// method, otherwise data races may occur.
func (rc *rejectionCache) Get(dir string) (bool, bool) {
	if rc == nil || rc.m == nil {
		return false, false
	}
	v, ok := rc.m[dir]
	return v, ok
}

// Store stores a new value for dir.  It is the callers responsibility to call
// rc.Lock and rc.Unlock before using this method, otherwise data races may
// occur.
func (rc *rejectionCache) Store(dir string, rejected bool) {
	if rc == nil {
		return
	}
	if rc.m == nil {
		rc.m = make(map[string]bool)
	}
	rc.m[dir] = rejected
}

// RejectByNameFunc is a function that takes a filename of a
// file that would be included in the backup. The function returns true if it
// should be excluded (rejected) from the backup.
type RejectByNameFunc func(path string) bool

// RejectFunc is a function that takes a filename and os.FileInfo of a
// file that would be included in the backup. The function returns true if it
// should be excluded (rejected) from the backup.
type RejectFunc func(path string, fi os.FileInfo) bool

// rejectByPattern returns a RejectByNameFunc which rejects files that match
// one of the patterns.
func rejectByPattern(patterns []string) RejectByNameFunc {
	return func(item string) bool {
		matched, _, err := filter.List(patterns, item)
		if err != nil {
			Warnf("error for exclude pattern: %v", err)
		}

		if matched {
			debug.Log("path %q excluded by an exclude pattern", item)
			return true
		}

		return false
	}
}

// rejectIfPresent returns a RejectByNameFunc which itself returns whether a path
// should be excluded. The RejectByNameFunc considers a file to be excluded when
// it resides in a directory with an exclusion file, that is specified by
// excludeFileSpec in the form "filename[:content]". The returned error is
// non-nil if the filename component of excludeFileSpec is empty. If rc is
// non-nil, it is going to be used in the RejectByNameFunc to expedite the evaluation
// of a directory based on previous visits.
func rejectIfPresent(excludeFileSpec string) (RejectByNameFunc, error) {
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
	rc := &rejectionCache{}
	fn := func(filename string) bool {
		return isExcludedByFile(filename, tf, tc, rc)
	}
	return fn, nil
}

// isExcludedByFile interprets filename as a path and returns true if that file
// is in a excluded directory. A directory is identified as excluded if it contains a
// tagfile which bears the name specified in tagFilename and starts with
// header. If rc is non-nil, it is used to expedite the evaluation of a
// directory based on previous visits.
func isExcludedByFile(filename, tagFilename, header string, rc *rejectionCache) bool {
	if tagFilename == "" {
		return false
	}
	dir, base := filepath.Split(filename)
	if base == tagFilename {
		return false // do not exclude the tagfile itself
	}
	rc.Lock()
	defer rc.Unlock()

	rejected, visited := rc.Get(dir)
	if visited {
		return rejected
	}
	rejected = isDirExcludedByFile(dir, tagFilename, header)
	rc.Store(dir, rejected)
	return rejected
}

func isDirExcludedByFile(dir, tagFilename, header string) bool {
	tf := filepath.Join(dir, tagFilename)
	_, err := fs.Lstat(tf)
	if os.IsNotExist(err) {
		return false
	}
	if err != nil {
		Warnf("could not access exclusion tagfile: %v", err)
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
	f, err := os.Open(tf)
	if err != nil {
		Warnf("could not open exclusion tagfile: %v", err)
		return false
	}
	defer f.Close()
	buf := make([]byte, len(header))
	_, err = io.ReadFull(f, buf)
	// EOF is handled with a dedicated message, otherwise the warning were too cryptic
	if err == io.EOF {
		Warnf("invalid (too short) signature in exclusion tagfile %q\n", tf)
		return false
	}
	if err != nil {
		Warnf("could not read signature from exclusion tagfile %q: %v\n", tf, err)
		return false
	}
	if bytes.Compare(buf, []byte(header)) != 0 {
		Warnf("invalid signature in exclusion tagfile %q\n", tf)
		return false
	}
	return true
}

// gatherDevices returns the set of unique device ids of the files and/or
// directory paths listed in "items".
func gatherDevices(items []string) (deviceMap map[string]uint64, err error) {
	deviceMap = make(map[string]uint64)
	for _, item := range items {
		item, err = filepath.Abs(filepath.Clean(item))
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

// rejectByDevice returns a RejectFunc that rejects files which are on a
// different file systems than the files/dirs in samples.
func rejectByDevice(samples []string) (RejectFunc, error) {
	allowed, err := gatherDevices(samples)
	if err != nil {
		return nil, err
	}
	debug.Log("allowed devices: %v\n", allowed)

	return func(item string, fi os.FileInfo) bool {
		if fi == nil {
			return false
		}

		item = filepath.Clean(item)

		id, err := fs.DeviceID(fi)
		if err != nil {
			// This should never happen because gatherDevices() would have
			// errored out earlier. If it still does that's a reason to panic.
			panic(err)
		}

		for dir := item; ; dir = filepath.Dir(dir) {
			debug.Log("item %v, test dir %v", item, dir)

			allowedID, ok := allowed[dir]
			if !ok {
				if dir == filepath.Dir(dir) {
					break
				}
				continue
			}

			if allowedID != id {
				debug.Log("path %q on disallowed device %d", item, id)
				return true
			}

			return false
		}

		panic(fmt.Sprintf("item %v, device id %v not found, allowedDevs: %v", item, id, allowed))
	}, nil
}

// rejectResticCache returns a RejectByNameFunc that rejects the restic cache
// directory (if set).
func rejectResticCache(repo *repository.Repository) (RejectByNameFunc, error) {
	if repo.Cache == nil {
		return func(string) bool {
			return false
		}, nil
	}
	cacheBase := repo.Cache.BaseDir()

	if cacheBase == "" {
		return nil, errors.New("cacheBase is empty string")
	}

	return func(item string) bool {
		if fs.HasPathPrefix(cacheBase, item) {
			debug.Log("rejecting restic cache directory %v", item)
			return true
		}

		return false
	}, nil
}
