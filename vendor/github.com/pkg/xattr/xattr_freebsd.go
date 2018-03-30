// +build freebsd

package xattr

import (
	"syscall"
)

const (
	EXTATTR_NAMESPACE_USER = 1
)

// Get retrieves extended attribute data associated with path.
func Get(path, name string) ([]byte, error) {
	// find size.
	size, err := extattr_get_file(path, EXTATTR_NAMESPACE_USER, name, nil, 0)
	if err != nil {
		return nil, &Error{"xattr.Get", path, name, err}
	}
	if size > 0 {
		buf := make([]byte, size)
		// Read into buffer of that size.
		read, err := extattr_get_file(path, EXTATTR_NAMESPACE_USER, name, &buf[0], size)
		if err != nil {
			return nil, &Error{"xattr.Get", path, name, err}
		}
		return buf[:read], nil
	}
	return []byte{}, nil
}

// List retrieves a list of names of extended attributes associated
// with the given path in the file system.
func List(path string) ([]string, error) {
	// find size.
	size, err := extattr_list_file(path, EXTATTR_NAMESPACE_USER, nil, 0)
	if err != nil {
		return nil, &Error{"xattr.List", path, "", err}
	}
	if size > 0 {
		buf := make([]byte, size)
		// Read into buffer of that size.
		read, err := extattr_list_file(path, EXTATTR_NAMESPACE_USER, &buf[0], size)
		if err != nil {
			return nil, &Error{"xattr.List", path, "", err}
		}
		return attrListToStrings(buf[:read]), nil
	}
	return []string{}, nil
}

// Set associates name and data together as an attribute of path.
func Set(path, name string, data []byte) error {
	var dataval *byte
	datalen := len(data)
	if datalen > 0 {
		dataval = &data[0]
	}
	written, err := extattr_set_file(path, EXTATTR_NAMESPACE_USER, name, dataval, datalen)
	if err != nil {
		return &Error{"xattr.Set", path, name, err}
	}
	if written != datalen {
		return &Error{"xattr.Set", path, name, syscall.E2BIG}
	}
	return nil
}

// Remove removes the attribute associated with the given path.
func Remove(path, name string) error {
	if err := extattr_delete_file(path, EXTATTR_NAMESPACE_USER, name); err != nil {
		return &Error{"xattr.Remove", path, name, err}
	}
	return nil
}

// Supported checks if filesystem supports extended attributes
func Supported(path string) bool {
	if _, err := extattr_list_file(path, EXTATTR_NAMESPACE_USER, nil, 0); err != nil {
		return err != syscall.ENOTSUP
	}
	return true
}

// attrListToStrings converts a sequnce of attribute name entries to a []string.
// Each entry consists of a single byte containing the length
// of the attribute name, followed by the attribute name.
// The name is _not_ terminated by NUL.
func attrListToStrings(buf []byte) []string {
	var result []string
	index := 0
	for index < len(buf) {
		next := index + 1 + int(buf[index])
		result = append(result, string(buf[index+1:next]))
		index = next
	}
	return result
}
