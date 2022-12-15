//go:build windows
// +build windows

package main

import (
	"fmt"
	"io"
	"os"
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

	return true
}

func nlink(info os.FileInfo) uint64 {
	return 1
}

func inode(info os.FileInfo) uint64 {
	return uint64(0)
}

func createFileSetPerHardlink(dir string) map[uint64][]string {
	linkTests := make(map[uint64][]string)
	files, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	for i, f := range files {
		linkTests[uint64(i)] = append(linkTests[uint64(i)], f.Name())
	}
	return linkTests
}
