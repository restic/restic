package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/filter"
	"github.com/restic/restic/internal/fs"
	"github.com/restic/restic/internal/repository"
)

// RejectFunc is a function that takes a filename and os.FileInfo of a
// file that would be included in the backup. The function returns true if it
// should be excluded (rejected) from the backup.
type RejectFunc func(path string, fi os.FileInfo) bool

// rejectByPattern returns a RejectFunc which rejects files that match
// one of the patterns.
func rejectByPattern(patterns []string) RejectFunc {
	return func(item string, fi os.FileInfo) bool {
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

// rejectIfPresent returns a RejectFunc which itself returns whether a path
// should be excluded. The RejectFunc considers a file to be excluded when
// it resides in a directory with an exclusion file, that is specified by
// excludeFileSpec in the form "filename[:content]". The returned error is
// non-nil if the filename component of excludeFileSpec is empty.
func rejectIfPresent(excludeFileSpec string) (RejectFunc, error) {
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
	fn := func(filename string, _ os.FileInfo) bool {
		return isExcludedByFile(filename, tf, tc)
	}
	return fn, nil
}

// isExcludedByFile interprets filename as a path and returns true if that file
// is in a excluded directory. A directory is identified as excluded if it contains a
// tagfile which bears the name specified in tagFilename and starts with header.
func isExcludedByFile(filename, tagFilename, header string) bool {
	if tagFilename == "" {
		return false
	}
	dir, base := filepath.Split(filename)
	if base == tagFilename {
		return false // do not exclude the tagfile itself
	}
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

		id, err := fs.DeviceID(fi)
		if err != nil {
			// This should never happen because gatherDevices() would have
			// errored out earlier. If it still does that's a reason to panic.
			panic(err)
		}

		for dir := item; dir != ""; dir = filepath.Dir(dir) {
			debug.Log("item %v, test dir %v", item, dir)

			allowedID, ok := allowed[dir]
			if !ok {
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

// rejectResticCache returns a RejectFunc that rejects the restic cache
// directory (if set).
func rejectResticCache(repo *repository.Repository) (RejectFunc, error) {
	if repo.Cache == nil {
		return func(string, os.FileInfo) bool {
			return false
		}, nil
	}
	cacheBase := repo.Cache.BaseDir()

	if cacheBase == "" {
		return nil, errors.New("cacheBase is empty string")
	}

	return func(item string, _ os.FileInfo) bool {
		if fs.HasPathPrefix(cacheBase, item) {
			debug.Log("rejecting restic cache directory %v", item)
			return true
		}

		return false
	}, nil
}
