package xattr

import (
	"syscall"
)

func get(path, attr string, dest []byte) (sz int, err error) {
	return syscall.Getxattr(path, attr, dest)
}

func list(path string, dest []byte) (sz int, err error) {
	return syscall.Listxattr(path, dest)
}

func set(path, attr string, data []byte, flags int) error {
	return syscall.Setxattr(path, attr, data, flags)
}

func remove(path, attr string) error {
	return syscall.Removexattr(path, attr)
}
