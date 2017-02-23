package xattr

// XAttrError records an error and the operation, file path and attribute that caused it.
type XAttrError struct {
	Op   string
	Path string
	Name string
	Err  error
}

func (e *XAttrError) Error() string {
	return e.Op + " " + e.Path + " " + e.Name + ": " + e.Err.Error()
}

// Convert an array of NULL terminated UTF-8 strings
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
