//go:build !linux
// +build !linux

package local

import "os"

func readdirnames(f *os.File) ([]string, error) {
	return f.Readdirnames(-1)
}
