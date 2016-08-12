package patchedos

import (
	"os"
	"restic/patched"
)

// Abstraction Layer to go's os package
// One reason is to able to patch around issues with windows long pathnames


func Chmod(name string, mode os.FileMode) error {
	return os.Chmod(patched.Fixpath(name), mode)
}

func Mkdir(name string, perm os.FileMode) error {
	return os.Mkdir(patched.Fixpath(name), perm)
}

func MkdirAll(path string, perm os.FileMode) error {
	return os.MkdirAll(patched.Fixpath(path), perm)
}

func Readlink(name string) (string, error) {
	return os.Readlink(patched.Fixpath(name))
}

func Remove(name string) error {
	return os.Remove(patched.Fixpath(name))
}

func RemoveAll(path string) error {
	return os.RemoveAll(patched.Fixpath(path))
}

func Rename(oldpath, newpath string) error {
	return os.Rename(patched.Fixpath(oldpath), patched.Fixpath(newpath))
}

func Symlink(oldname, newname string) error {
	return os.Symlink(patched.Fixpath(oldname), patched.Fixpath(newname))
}

func Stat(name string) (os.FileInfo, error) {
	return os.Stat(patched.Fixpath(name))
}

func Lstat(name string) (os.FileInfo, error) {
	return os.Lstat(patched.Fixpath(name))
}

func Open(name string) (*os.File, error) {
	return os.Open(patched.Fixpath(name))
}

func Create(name string) (*os.File, error) {
	return os.Create(patched.Fixpath(name))
}

func OpenFile(name string, flag int, perm os.FileMode) (*os.File, error) {
	return os.OpenFile(patched.Fixpath(name), flag, perm)
}
