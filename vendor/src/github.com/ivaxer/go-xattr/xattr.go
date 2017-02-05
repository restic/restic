// Package xattr provides a simple interface to user extended attributes on
// Linux and OSX. Support for xattrs is filesystem dependant, so not a given
// even if you are running one of those operating systems.
//
// On Linux you have to edit /etc/fstab to include "user_xattr". Also, on Linux
// user's extended attributes have a manditory prefix of "user.".
package xattr

import (
	"os"
)

// IsNotExist returns a boolean indicating whether the error is known to report
// that an extended attribute does not exist.
func IsNotExist(err error) bool {
	if e, ok := err.(*os.PathError); ok {
		err = e.Err
	}

	return isNotExist(err)
}

// Converts an array of NUL terminated UTF-8 strings
// to a []string.
func nullTermToStrings(buf []byte) (result []string) {
	offset := 0
	for index, b := range buf {
		if b == 0 {
			result = append(result, string(buf[offset:index]))
			offset = index + 1
		}
	}
	return
}

// Getxattr retrieves value of the extended attribute identified by attr
// associated with given path in filesystem into buffer dest.
//
// On success, dest contains data associated with attr, retrieved value size sz
// and nil error are returned.
//
// On error, non-nil error is returned. Getxattr returns error if dest was too
// small for attribute value.
//
// A nil slice can be passed as dest to get current size of attribute value,
// which can be used to estimate dest length for value associated with attr.
//
// See getxattr(2) for more information.
//
// Get is high-level function on top of Getxattr. Getxattr more efficient,
// because it issues one syscall per call, doesn't allocate memory for
// attribute data (caller can reuse buffer).
func Getxattr(path, attr string, dest []byte) (sz int, err error) {
	return get(path, attr, dest)
}

// Get retrieves extended attribute data associated with path. If there is an
// error, it will be of type *os.PathError.
//
// See Getxattr for low-level usage.
func Get(path, attr string) ([]byte, error) {
	// find size
	size, err := Getxattr(path, attr, nil)
	if err != nil {
		return nil, &os.PathError{"getxattr", path, err}
	}
	if size == 0 {
		return []byte{}, nil
	}

	// read into buffer of that size
	buf := make([]byte, size)
	size, err = Getxattr(path, attr, buf)
	if err != nil {
		return nil, &os.PathError{"getxattr", path, err}
	}
	return buf[:size], nil
}

// Listxattr retrieves the list of extended attribute names associated with
// path. The list is set of NULL-terminated names.
//
// On success, dest containes list of NULL-terminated names, the length of the
// extended attribute list and nil error are returned.
//
// On error, non nil error is returned. Listxattr returns error if dest buffer
// was too small for extended attribute list.
//
// The list of names is returned as an unordered array of NULL-terminated
// character strings (attribute names are separated by NULL characters), like
// this:
//     user.name1\0system.name1\0user.name2\0
//
// A nil slice can be passed as dest to get the current size of the list of
// extended attribute names, which can be used to estimate dest length for
// the list of names.
//
// See listxattr(2) for more information.
//
// List is high-level function on top of Listxattr.
func Listxattr(path string, dest []byte) (sz int, err error) {
	return list(path, dest)
}

// List retrieves a list of names of extended attributes associated with path.
// If there is an error, it will be of type *os.PathError.
//
// See Listxattr for low-level usage.
func List(path string) ([]string, error) {
	// find size
	size, err := Listxattr(path, nil)
	if err != nil {
		return nil, &os.PathError{"listxattr", path, err}
	}
	if size == 0 {
		return []string{}, nil
	}

	// read into buffer of that size
	buf := make([]byte, size)
	size, err = Listxattr(path, buf)
	if err != nil {
		return nil, &os.PathError{"listxattr", path, err}
	}
	return nullTermToStrings(buf[:size]), nil
}

// Setxattr sets value in data of extended attribute attr and accosiated with
// path.
//
// The flags refine the semantic of the operation. XATTR_CREATE specifies pure
// create, which fails if attr already exists. XATTR_REPLACE  specifies a pure
// replace operation, which fails if the attr does not already exist. By
// default (no flags), the attr will be created if need be, or will simply
// replace the value if attr exists.
//
// On error, non nil error is returned.
//
// See setxattr(2) for more information.
func Setxattr(path, attr string, data []byte, flags int) error {
	return set(path, attr, data, flags)
}

// Set associates data as an extended attribute of path. If there is an error,
// it will be of type *os.PathError.
//
// See Setxattr for low-level usage.
func Set(path, attr string, data []byte) error {
	if err := Setxattr(path, attr, data, 0); err != nil {
		return &os.PathError{"setxattr", path, err}
	}
	return nil
}

// Removexattr removes the extended attribute attr accosiated with path.
//
// On error, non-nil error is returned.
//
// See removexattr(2) for more information.
func Removexattr(path, attr string) error {
	return remove(path, attr)
}

// Remove removes the extended attribute. If there is an error, it will be of
// type *os.PathError.
func Remove(path, attr string) error {
	if err := Removexattr(path, attr); err != nil {
		return &os.PathError{"removexattr", path, err}
	}
	return nil
}
