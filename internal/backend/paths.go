package backend

import "os"

type Modes struct {
	Dir  os.FileMode
	File os.FileMode
}

// DefaultModes defines the default permissions to apply to new repository
// files and directories stored on file-based backends.
var DefaultModes = Modes{Dir: 0700, File: 0600}

// DeriveModesFromFileInfo will, given the mode of a regular file, compute
// the mode we should use for new files and directories. If the passed
// error is non-nil DefaultModes are returned.
func DeriveModesFromFileInfo(fi os.FileInfo, err error) Modes {
	m := DefaultModes
	if err != nil {
		return m
	}

	if fi.Mode()&0040 != 0 { // Group has read access
		m.Dir |= 0070
		m.File |= 0060
	}

	return m
}
