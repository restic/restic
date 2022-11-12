package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/filter"
	"github.com/restic/restic/internal/fs"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/textfile"
	"github.com/spf13/pflag"
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
	parsedPatterns := filter.ParsePatterns(patterns)
	return func(item string) bool {
		matched, err := filter.List(parsedPatterns, item)
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

// Same as `rejectByPattern` but case insensitive.
func rejectByInsensitivePattern(patterns []string) RejectByNameFunc {
	for index, path := range patterns {
		patterns[index] = strings.ToLower(path)
	}

	rejFunc := rejectByPattern(patterns)
	return func(item string) bool {
		return rejFunc(strings.ToLower(item))
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
// is in an excluded directory. A directory is identified as excluded if it contains a
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
	defer func() {
		_ = f.Close()
	}()
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
	if !bytes.Equal(buf, []byte(header)) {
		Warnf("invalid signature in exclusion tagfile %q\n", tf)
		return false
	}
	return true
}

// DeviceMap is used to track allowed source devices for backup. This is used to
// check for crossing mount points during backup (for --one-file-system). It
// maps the name of a source path to its device ID.
type DeviceMap map[string]uint64

// NewDeviceMap creates a new device map from the list of source paths.
func NewDeviceMap(allowedSourcePaths []string) (DeviceMap, error) {
	deviceMap := make(map[string]uint64)

	for _, item := range allowedSourcePaths {
		item, err := filepath.Abs(filepath.Clean(item))
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
func (m DeviceMap) IsAllowed(item string, deviceID uint64) (bool, error) {
	for dir := item; ; dir = filepath.Dir(dir) {
		debug.Log("item %v, test dir %v", item, dir)

		// find a parent directory that is on an allowed device (otherwise
		// we would not traverse the directory at all)
		allowedID, ok := m[dir]
		if !ok {
			if dir == filepath.Dir(dir) {
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

// rejectByDevice returns a RejectFunc that rejects files which are on a
// different file systems than the files/dirs in samples.
func rejectByDevice(samples []string) (RejectFunc, error) {
	deviceMap, err := NewDeviceMap(samples)
	if err != nil {
		return nil, err
	}
	debug.Log("allowed devices: %v\n", deviceMap)

	return func(item string, fi os.FileInfo) bool {
		id, err := fs.DeviceID(fi)
		if err != nil {
			// This should never happen because gatherDevices() would have
			// errored out earlier. If it still does that's a reason to panic.
			panic(err)
		}

		allowed, err := deviceMap.IsAllowed(filepath.Clean(item), id)
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
		parentDir := filepath.Dir(filepath.Clean(item))

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

		parentAllowed, err := deviceMap.IsAllowed(parentDir, parentDeviceID)
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

func rejectBySize(maxSizeStr string) (RejectFunc, error) {
	maxSize, err := parseSizeStr(maxSizeStr)
	if err != nil {
		return nil, err
	}

	return func(item string, fi os.FileInfo) bool {
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

func parseSizeStr(sizeStr string) (int64, error) {
	if sizeStr == "" {
		return 0, errors.New("expected size, got empty string")
	}

	numStr := sizeStr[:len(sizeStr)-1]
	var unit int64 = 1

	switch sizeStr[len(sizeStr)-1] {
	case 'b', 'B':
		// use initialized values, do nothing here
	case 'k', 'K':
		unit = 1024
	case 'm', 'M':
		unit = 1024 * 1024
	case 'g', 'G':
		unit = 1024 * 1024 * 1024
	case 't', 'T':
		unit = 1024 * 1024 * 1024 * 1024
	default:
		numStr = sizeStr
	}
	value, err := strconv.ParseInt(numStr, 10, 64)
	if err != nil {
		return 0, err
	}
	return value * unit, nil
}

// readExcludePatternsFromFiles reads all exclude files and returns the list of
// exclude patterns. For each line, leading and trailing white space is removed
// and comment lines are ignored. For each remaining pattern, environment
// variables are resolved. For adding a literal dollar sign ($), write $$ to
// the file.
func readExcludePatternsFromFiles(excludeFiles []string) ([]string, error) {
	getenvOrDollar := func(s string) string {
		if s == "$" {
			return "$"
		}
		return os.Getenv(s)
	}

	var excludes []string
	for _, filename := range excludeFiles {
		err := func() (err error) {
			data, err := textfile.Read(filename)
			if err != nil {
				return err
			}

			scanner := bufio.NewScanner(bytes.NewReader(data))
			for scanner.Scan() {
				line := strings.TrimSpace(scanner.Text())

				// ignore empty lines
				if line == "" {
					continue
				}

				// strip comments
				if strings.HasPrefix(line, "#") {
					continue
				}

				line = os.Expand(line, getenvOrDollar)
				excludes = append(excludes, line)
			}
			return scanner.Err()
		}()
		if err != nil {
			return nil, err
		}
	}
	return excludes, nil
}

type excludePatternOptions struct {
	Excludes                []string
	InsensitiveExcludes     []string
	ExcludeFiles            []string
	InsensitiveExcludeFiles []string
}

func initExcludePatternOptions(f *pflag.FlagSet, opts *excludePatternOptions) {
	f.StringArrayVarP(&opts.Excludes, "exclude", "e", nil, "exclude a `pattern` (can be specified multiple times)")
	f.StringArrayVar(&opts.InsensitiveExcludes, "iexclude", nil, "same as --exclude `pattern` but ignores the casing of filenames")
	f.StringArrayVar(&opts.ExcludeFiles, "exclude-file", nil, "read exclude patterns from a `file` (can be specified multiple times)")
	f.StringArrayVar(&opts.InsensitiveExcludeFiles, "iexclude-file", nil, "same as --exclude-file but ignores casing of `file`names in patterns")
}

func (opts *excludePatternOptions) Empty() bool {
	return len(opts.Excludes) == 0 && len(opts.InsensitiveExcludes) == 0 && len(opts.ExcludeFiles) == 0 && len(opts.InsensitiveExcludeFiles) == 0
}

func (opts excludePatternOptions) CollectPatterns() ([]RejectByNameFunc, error) {
	var fs []RejectByNameFunc
	// add patterns from file
	if len(opts.ExcludeFiles) > 0 {
		excludePatterns, err := readExcludePatternsFromFiles(opts.ExcludeFiles)
		if err != nil {
			return nil, err
		}

		if err := filter.ValidatePatterns(excludePatterns); err != nil {
			return nil, errors.Fatalf("--exclude-file: %s", err)
		}

		opts.Excludes = append(opts.Excludes, excludePatterns...)
	}

	if len(opts.InsensitiveExcludeFiles) > 0 {
		excludes, err := readExcludePatternsFromFiles(opts.InsensitiveExcludeFiles)
		if err != nil {
			return nil, err
		}

		if err := filter.ValidatePatterns(excludes); err != nil {
			return nil, errors.Fatalf("--iexclude-file: %s", err)
		}

		opts.InsensitiveExcludes = append(opts.InsensitiveExcludes, excludes...)
	}

	if len(opts.InsensitiveExcludes) > 0 {
		if err := filter.ValidatePatterns(opts.InsensitiveExcludes); err != nil {
			return nil, errors.Fatalf("--iexclude: %s", err)
		}

		fs = append(fs, rejectByInsensitivePattern(opts.InsensitiveExcludes))
	}

	if len(opts.Excludes) > 0 {
		if err := filter.ValidatePatterns(opts.Excludes); err != nil {
			return nil, errors.Fatalf("--exclude: %s", err)
		}

		fs = append(fs, rejectByPattern(opts.Excludes))
	}
	return fs, nil
}
