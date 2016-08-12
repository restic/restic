package patchedos

import (
	"os"
	"runtime"
)

// Abstraction Layer to go's os package
// One reason is to able to patch around issues with windows long pathnames

func Fixpath(name string) string {
	if runtime.GOOS == "windows" {
		return "\\\\?\\" + name
	} else {
		return name
	}
}

func Chmod(name string, mode os.FileMode) error {
	return os.Chmod(Fixpath(name), mode)
}

func Mkdir(name string, perm os.FileMode) error {
	return os.Mkdir(Fixpath(name), perm)
}

func MkdirAll(path string, perm os.FileMode) error {
	return os.MkdirAll(Fixpath(path), perm)
}

func Readlink(name string) (string, error) {
	return os.Readlink(Fixpath(name))
}

func Remove(name string) error {
	return os.Remove(Fixpath(name))
}

func RemoveAll(path string) error {
	return os.RemoveAll(Fixpath(path))
}

func Rename(oldpath, newpath string) error {
	return os.Rename(Fixpath(oldpath), Fixpath(newpath))
}

func Symlink(oldname, newname string) error {
	return os.Symlink(Fixpath(oldname), Fixpath(newname))
}

func Stat(name string) (os.FileInfo, error) {
	return os.Stat(Fixpath(name))
}

func Lstat(name string) (os.FileInfo, error) {
	return os.Lstat(Fixpath(name))
}

func Open(name string) (*os.File, error) {
	return os.Open(Fixpath(name))
}

func Create(name string) (*os.File, error) {
	return os.Create(Fixpath(name))
}

func OpenFile(name string, flag int, perm os.FileMode) (*os.File, error) {
	return os.OpenFile(Fixpath(name), flag, perm)
}
