package fs

import "os"

// TODO macOS versions >= 13 support freadlink. Use that instead of the fallback codepath

func Freadlink(fd uintptr, name string) (string, error) {
	return os.Readlink(name)
}
