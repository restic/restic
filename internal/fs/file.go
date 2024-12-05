package fs

import (
	"fmt"
	"github.com/restic/restic/internal/errors"
	"golang.org/x/sys/unix"
	"io"
	"os"
	"reflect"
	"runtime"
	"sync"
)

type cloneMethod func(src, dst *os.File) error

var (
	mCloneMethods = &sync.Mutex{}
	cloneMethods  = make([]cloneMethod, 0, 1)
)

func registerCloneMethod(method cloneMethod) {
	mCloneMethods.Lock()
	defer mCloneMethods.Unlock()

	cloneMethods = append(cloneMethods, method)
	if len(cloneMethods) > 1 {
		var names []string
		for _, m := range cloneMethods {
			names = append(names, runtime.FuncForPC(reflect.ValueOf(m).Pointer()).Name())
		}
		fmt.Fprintf(os.Stderr, "warning: more than one clone method: %v\n", names)
	}
}

// MkdirAll creates a directory named path, along with any necessary parents,
// and returns nil, or else returns an error. The permission bits perm are used
// for all directories that MkdirAll creates. If path is already a directory,
// MkdirAll does nothing and returns nil.
func MkdirAll(path string, perm os.FileMode) error {
	return os.MkdirAll(fixpath(path), perm)
}

// Remove removes the named file or directory.
// If there is an error, it will be of type *PathError.
func Remove(name string) error {
	return os.Remove(fixpath(name))
}

// RemoveAll removes path and any children it contains.
// It removes everything it can but returns the first error
// it encounters.  If the path does not exist, RemoveAll
// returns nil (no error).
func RemoveAll(path string) error {
	return os.RemoveAll(fixpath(path))
}

// Link creates newname as a hard link to oldname.
// If there is an error, it will be of type *LinkError.
func Link(oldname, newname string) error {
	return os.Link(fixpath(oldname), fixpath(newname))
}

// Lstat returns the FileInfo structure describing the named file.
// If the file is a symbolic link, the returned FileInfo
// describes the symbolic link.  Lstat makes no attempt to follow the link.
// If there is an error, it will be of type *PathError.
func Lstat(name string) (os.FileInfo, error) {
	return os.Lstat(fixpath(name))
}

// OpenFile is the generalized open call; most users will use Open
// or Create instead.  It opens the named file with specified flag
// (O_RDONLY etc.) and perm, (0666 etc.) if applicable.  If successful,
// methods on the returned File can be used for I/O.
// If there is an error, it will be of type *PathError.
func OpenFile(name string, flag int, perm os.FileMode) (*os.File, error) {
	if runtime.GOOS == "windows" {
		flag &^= O_NOFOLLOW
	}
	return os.OpenFile(fixpath(name), flag, perm)
}

// IsAccessDenied checks if the error is due to permission error.
func IsAccessDenied(err error) bool {
	return os.IsPermission(err)
}

// ResetPermissions resets the permissions of the file at the specified path
func ResetPermissions(path string) error {
	// Set the default file permissions
	if err := os.Chmod(fixpath(path), 0600); err != nil {
		return err
	}
	return nil
}

// Readdirnames returns a list of file in a directory. Flags are passed to fs.OpenFile.
// O_RDONLY and O_DIRECTORY are implied.
func Readdirnames(filesystem FS, dir string, flags int) ([]string, error) {
	f, err := filesystem.OpenFile(dir, O_RDONLY|O_DIRECTORY|flags, false)
	if err != nil {
		return nil, fmt.Errorf("openfile for readdirnames failed: %w", err)
	}

	entries, err := f.Readdirnames(-1)
	if err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("readdirnames %v failed: %w", dir, err)
	}

	err = f.Close()
	if err != nil {
		return nil, err
	}

	return entries, nil
}

func doCloneCopy(src, dest *os.File) error {
	srcInfo, err := src.Stat()
	if err != nil {
		return err
	}
	n, err := io.Copy(dest, src)
	if n > 0 && n != srcInfo.Size() {
		return errors.Wrapf(err, "io.Copy() wrote %d of %d bytes", n, srcInfo.Size())
	}
	return err
}

func doClone(srcName, destName string, method cloneMethod) error {
	src, err := OpenFile(srcName, O_RDONLY|O_NOFOLLOW, 0)
	if err != nil {
		return err
	}
	defer func() {
		_ = src.Close()
	}()

	dest, err := OpenFile(destName, O_CREATE|O_TRUNC|O_WRONLY|O_NOFOLLOW, 0600)
	if err != nil {
		return err
	}
	defer func() {
		_ = dest.Close()
	}()

	_ = src.Sync()
	return method(src, dest)
}

// Clone performs a local possibly accelerated copy of srcName to destName.
// The cloned flag reports whether an accelerated copy (reflink) was performed.
func Clone(srcName, destName string) (cloned bool, err error) {
	for _, fn := range cloneMethods {
		err = doClone(srcName, destName, fn)
		// if a particular method is not supported, or we hit the cross-device limitation,
		// "eat" the error and go to the next method or the fallback
		if errors.Is(err, unix.EXDEV) || errors.Is(err, unix.ENOTSUP) {
			continue
		}
		return true, err
	}

	return false, doClone(srcName, destName, doCloneCopy)
}
