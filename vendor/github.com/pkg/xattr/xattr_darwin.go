// +build darwin

package xattr

import "syscall"

// Get retrieves extended attribute data associated with path.
func Get(path, name string) ([]byte, error) {
	// find size.
	size, err := getxattr(path, name, nil, 0, 0, 0)
	if err != nil {
		return nil, &Error{"xattr.Get", path, name, err}
	}
	if size > 0 {
		buf := make([]byte, size)
		// Read into buffer of that size.
		read, err := getxattr(path, name, &buf[0], size, 0, 0)
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
	size, err := listxattr(path, nil, 0, 0)
	if err != nil {
		return nil, &Error{"xattr.List", path, "", err}
	}
	if size > 0 {
		buf := make([]byte, size)
		// Read into buffer of that size.
		read, err := listxattr(path, &buf[0], size, 0)
		if err != nil {
			return nil, &Error{"xattr.List", path, "", err}
		}
		return nullTermToStrings(buf[:read]), nil
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
	if err := setxattr(path, name, dataval, datalen, 0, 0); err != nil {
		return &Error{"xattr.Set", path, name, err}
	}
	return nil
}

// Remove removes the attribute associated with the given path.
func Remove(path, name string) error {
	if err := removexattr(path, name, 0); err != nil {
		return &Error{"xattr.Remove", path, name, err}
	}
	return nil
}

// Supported checks if filesystem supports extended attributes
func Supported(path string) bool {
	if _, err := listxattr(path, nil, 0, 0); err != nil {
		return err != syscall.ENOTSUP
	}
	return true
}
