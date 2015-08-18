//+build windows

package main

import (
	"fmt"
	"os"
)

func (e *dirEntry) equals(other *dirEntry) bool {
	if e.path != other.path {
		fmt.Fprintf(os.Stderr, "%v: path does not match (%v != %v)\n", e.path, e.path, other.path)
		return false
	}

	if e.fi.Mode() != other.fi.Mode() {
		fmt.Fprintf(os.Stderr, "%v: mode does not match (%v != %v)\n", e.path, e.fi.Mode(), other.fi.Mode())
		return false
	}

	if !sameModTime(e.fi, other.fi) {
		fmt.Fprintf(os.Stderr, "%v: ModTime does not match (%v != %v)\n", e.path, e.fi.ModTime(), other.fi.ModTime())
		return false
	}

	return true
}
