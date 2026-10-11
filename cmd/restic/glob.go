// Copyright 2010 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
//
// Derived from Go's path/filepath glob (src/path/filepath/match.go).
// Modified so that directory components matched by a wildcard are not
// followed when they are symbolic links.

package main

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
)

// globNoFollow is like filepath.Glob, but does not descend into directory
// symlinks that were matched by a wildcard. Literal path components,
// including a directory symlink named after a wildcard (for example
// tmp/*/foo when foo is a symlink to a directory), are still followed.
// Literal path prefixes that are themselves symlinks are also followed
// (so a pattern like "symlink/*.txt" keeps working). This prevents
// Wine-style dosdevices/z: links from expanding include patterns into
// /proc and exhausting memory (#21790).
func globNoFollow(pattern string) ([]string, error) {
	return globNoFollowLimit(pattern, 0, true)
}

func globNoFollowLimit(pattern string, depth int, followDir bool) ([]string, error) {
	// Same recursion cap as path/filepath.Glob (CVE-2022-30632).
	const pathSeparatorsLimit = 10000
	if depth == pathSeparatorsLimit {
		return nil, filepath.ErrBadPattern
	}

	if _, err := filepath.Match(pattern, ""); err != nil {
		return nil, err
	}
	if !globHasMeta(pattern) {
		if _, err := os.Lstat(pattern); err != nil {
			return nil, nil
		}
		return []string{pattern}, nil
	}

	dir, file := filepath.Split(pattern)
	volumeLen := 0
	if runtime.GOOS == "windows" {
		volumeLen, dir = cleanGlobPathWindows(dir)
	} else {
		dir = cleanGlobPath(dir)
	}

	if !globHasMeta(dir[volumeLen:]) {
		return globInDir(dir, file, followDir)
	}
	if dir == pattern {
		return nil, filepath.ErrBadPattern
	}

	parents, err := globNoFollowLimit(dir, depth+1, followDir)
	if err != nil {
		return nil, err
	}
	// followDir is per-component: only wildcard-matched directory
	// components use Lstat (do not follow). A literal component after a
	// wildcard still uses Stat, matching filepath.Glob for non-wildcard dirs.
	_, component := filepath.Split(dir)
	followComponent := !globHasMeta(component)
	var matches []string
	for _, parent := range parents {
		m, err := globInDir(parent, file, followComponent)
		if err != nil {
			return nil, err
		}
		matches = append(matches, m...)
	}
	return matches, nil
}

func globInDir(dir, pattern string, followDir bool) ([]string, error) {
	var (
		fi  os.FileInfo
		err error
	)
	if followDir {
		fi, err = os.Stat(dir)
	} else {
		fi, err = os.Lstat(dir)
	}
	if err != nil || !fi.IsDir() {
		return nil, nil
	}
	d, err := os.Open(dir)
	if err != nil {
		return nil, nil
	}
	defer d.Close()

	names, _ := d.Readdirnames(-1)
	slices.Sort(names)

	var matches []string
	for _, n := range names {
		ok, err := filepath.Match(pattern, n)
		if err != nil {
			return matches, err
		}
		if ok {
			matches = append(matches, filepath.Join(dir, n))
		}
	}
	return matches, nil
}

func cleanGlobPath(path string) string {
	switch path {
	case "":
		return "."
	case string(filepath.Separator):
		return path
	default:
		return path[:len(path)-1]
	}
}

func cleanGlobPathWindows(path string) (prefixLen int, cleaned string) {
	vollen := len(filepath.VolumeName(path))
	switch {
	case path == "":
		return 0, "."
	case vollen+1 == len(path) && os.IsPathSeparator(path[len(path)-1]):
		return vollen + 1, path
	case vollen == len(path) && len(path) == 2:
		return vollen, path + "."
	default:
		if vollen >= len(path) {
			vollen = len(path) - 1
		}
		return vollen, path[:len(path)-1]
	}
}

func globHasMeta(path string) bool {
	magicChars := `*?[`
	if runtime.GOOS != "windows" {
		magicChars = `*?[\`
	}
	return strings.ContainsAny(path, magicChars)
}
