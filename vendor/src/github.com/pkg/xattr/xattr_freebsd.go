// +build freebsd

package xattr

import (
	"syscall"
)

const (
	EXTATTR_NAMESPACE_USER = 1
)

// Getxattr retrieves extended attribute data associated with path.
func Getxattr(path, name string) ([]byte, error) {
	// find size.
	size, err := extattr_get_file(path, EXTATTR_NAMESPACE_USER, name, nil, 0)
	if err != nil {
		return nil, &XAttrError{"extattr_get_file", path, name, err}
	}
	if size > 0 {
		buf := make([]byte, size)
		// Read into buffer of that size.
		read, err := extattr_get_file(path, EXTATTR_NAMESPACE_USER, name, &buf[0], size)
		if err != nil {
			return nil, &XAttrError{"extattr_get_file", path, name, err}
		}
		return buf[:read], nil
	}
	return []byte{}, nil
}

// Listxattr retrieves a list of names of extended attributes associated
// with the given path in the file system.
func Listxattr(path string) ([]string, error) {
	// find size.
	size, err := extattr_list_file(path, EXTATTR_NAMESPACE_USER, nil, 0)
	if err != nil {
		return nil, &XAttrError{"extattr_list_file", path, "", err}
	}
	if size > 0 {
		buf := make([]byte, size)
		// Read into buffer of that size.
		read, err := extattr_list_file(path, EXTATTR_NAMESPACE_USER, &buf[0], size)
		if err != nil {
			return nil, &XAttrError{"extattr_list_file", path, "", err}
		}
		return attrListToStrings(buf[:read]), nil
	}
	return []string{}, nil
}

// Setxattr associates name and data together as an attribute of path.
func Setxattr(path, name string, data []byte) error {
	written, err := extattr_set_file(path, EXTATTR_NAMESPACE_USER, name, &data[0], len(data))
	if err != nil {
		return &XAttrError{"extattr_set_file", path, name, err}
	}
	if written != len(data) {
		return &XAttrError{"extattr_set_file", path, name, syscall.E2BIG}
	}
	return nil
}

// Removexattr removes the attribute associated with the given path.
func Removexattr(path, name string) error {
	if err := extattr_delete_file(path, EXTATTR_NAMESPACE_USER, name); err != nil {
		return &XAttrError{"extattr_delete_file", path, name, err}
	}
	return nil
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
