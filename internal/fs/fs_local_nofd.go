//go:build !linux && !darwin && !windows

package fs

func newFdLocalFile(name string, flag int, metadataOnly bool) (File, error) {
	panic("not supported")
}
