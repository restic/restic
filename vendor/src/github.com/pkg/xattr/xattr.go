/*
Package xattr provides support for extended attributes on linux, darwin and freebsd.
Extended attributes are name:value pairs associated permanently with files and directories,
similar to the environment strings associated with a process.
An attribute may be defined or undefined. If it is defined, its value may be empty or non-empty.
More details you can find here: https://en.wikipedia.org/wiki/Extended_file_attributes
*/
package xattr

// Error records an error and the operation, file path and attribute that caused it.
type Error struct {
	Op   string
	Path string
	Name string
	Err  error
}

func (e *Error) Error() string {
	return e.Op + " " + e.Path + " " + e.Name + ": " + e.Err.Error()
}

// nullTermToStrings converts an array of NULL terminated UTF-8 strings to a []string.
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
