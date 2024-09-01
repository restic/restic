//go:build !windows
// +build !windows

package fs

import (
	"os"
)

func lchown(name string, uid, gid int) error {
	return os.Lchown(name, uid, gid)
}
