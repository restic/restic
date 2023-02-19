//go:build !windows
// +build !windows

package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
)

func (e *dirEntry) equals(out io.Writer, other *dirEntry) bool {
	if e.path != other.path {
		fmt.Fprintf(out, "%v: path does not match (%v != %v)\n", e.path, e.path, other.path)
		return false
	}

	if e.fi.Mode() != other.fi.Mode() {
		fmt.Fprintf(out, "%v: mode does not match (%v != %v)\n", e.path, e.fi.Mode(), other.fi.Mode())
		return false
	}

	if !sameModTime(e.fi, other.fi) {
		fmt.Fprintf(out, "%v: ModTime does not match (%v != %v)\n", e.path, e.fi.ModTime(), other.fi.ModTime())
		return false
	}

	stat, _ := e.fi.Sys().(*syscall.Stat_t)
	stat2, _ := other.fi.Sys().(*syscall.Stat_t)

	if stat.Uid != stat2.Uid {
		fmt.Fprintf(out, "%v: UID does not match (%v != %v)\n", e.path, stat.Uid, stat2.Uid)
		return false
	}

	if stat.Gid != stat2.Gid {
		fmt.Fprintf(out, "%v: GID does not match (%v != %v)\n", e.path, stat.Gid, stat2.Gid)
		return false
	}

	if stat.Nlink != stat2.Nlink {
		fmt.Fprintf(out, "%v: Number of links do not match (%v != %v)\n", e.path, stat.Nlink, stat2.Nlink)
		return false
	}

	return true
}

func nlink(info os.FileInfo) uint64 {
	stat, _ := info.Sys().(*syscall.Stat_t)
	return uint64(stat.Nlink)
}

func createFileSetPerHardlink(dir string) map[uint64][]string {
	var stat syscall.Stat_t
	linkTests := make(map[uint64][]string)
	files, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	for _, f := range files {

		if err := syscall.Stat(filepath.Join(dir, f.Name()), &stat); err != nil {
			return nil
		}
		linkTests[uint64(stat.Ino)] = append(linkTests[uint64(stat.Ino)], f.Name())
	}
	return linkTests
}
