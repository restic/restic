package restorer

import "os"

// OpenFile opens the file with truncate and write only options.
// In case of windows, it first attempts to open an existing file,
// and only if the file does not exist, it opens it with create option
// in order to create the file. This is done, otherwise if the ads stream
// is written first (in which case it automatically creates an empty main
// file before writing the stream) and then when the main file is written
// later, the ads stream can be overwritten.
func (*filesWriter) OpenFile(createSize int64, path string) (*os.File, error) {
	var flags int
	var f *os.File
	var err error
	// TODO optimize this. When GenericAttribute change is merged, we can
	// leverage that here. If a file has ADS, we will have a GenericAttribute of
	// type TypeADS added in the file with values as a string of ads stream names
	// and we will do the following only if the TypeADS attribute is found in the node.
	// Otherwise we will directly just use the create option while opening the file.
	if createSize >= 0 {
		flags = os.O_TRUNC | os.O_WRONLY
		f, err = os.OpenFile(path, flags, 0600)
		if err != nil && os.IsNotExist(err) {
			//If file not exists open with create flag
			flags = os.O_CREATE | os.O_TRUNC | os.O_WRONLY
			f, err = os.OpenFile(path, flags, 0600)
		}
	} else {
		flags = os.O_WRONLY
		f, err = os.OpenFile(path, flags, 0600)
	}

	return f, err
}
