// +build !linux,!freebsd,!netbsd,!darwin

package xattr

import (
	"os"
)

func getxattr(path string, name string, data []byte) (int, error) {
	return 0, nil
}

func lgetxattr(path string, name string, data []byte) (int, error) {
	return 0, nil
}

func fgetxattr(f *os.File, name string, data []byte) (int, error) {
	return 0, nil
}

func setxattr(path string, name string, data []byte, flags int) error {
	return nil
}

func lsetxattr(path string, name string, data []byte, flags int) error {
	return nil
}

func fsetxattr(f *os.File, name string, data []byte, flags int) error {
	return nil
}

func removexattr(path string, name string) error {
	return nil
}

func lremovexattr(path string, name string) error {
	return nil
}

func fremovexattr(f *os.File, name string) error {
	return nil
}

func listxattr(path string, data []byte) (int, error) {
	return 0, nil
}

func llistxattr(path string, data []byte) (int, error) {
	return 0, nil
}

func flistxattr(f *os.File, data []byte) (int, error) {
	return 0, nil
}

// dummy
func stringsFromByteSlice(buf []byte) (result []string) {
	return []string{}
}
