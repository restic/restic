package local

import (
	"os"
	"syscall"
	"unsafe"
)

// Read all dir entries, including even entries with st_ino == 0
// Works around bugs in FUSE's synthetic inode number generation.

func parseDirent(buf []byte) (consumed int, name string, err error) {
	dirent := (*syscall.Dirent)(unsafe.Pointer(&buf[0]))
	nameBytes := (*[256]byte)(unsafe.Pointer(&dirent.Name[0]))
	for i := 0; i < 256; i++ {
		if nameBytes[i] == 0 {
			name = string(nameBytes[:i])
			break
		}
	}
	return int(dirent.Reclen), name, nil
}

func readdirnames(f *os.File) ([]string, error) {
	buf := make([]byte, 8192)
	names := []string{}
	for {
		bytes, err := syscall.Getdents(int(f.Fd()), buf)
		if err != nil {
			return nil, err
		}
		if bytes == 0 {
			break
		}
		for n := 0; n < bytes; {
			consumed, name, err := parseDirent(buf[n:])
			if err != nil {
				return nil, err
			}
			if name != ".." && name != "." {
				names = append(names, name)
			}
			n += consumed
		}
	}
	return names, nil
}
