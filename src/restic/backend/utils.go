package backend

import (
	"io"

	"github.com/pkg/errors"
)

// LoadAll reads all data stored in the backend for the handle. The buffer buf
// is resized to accomodate all data in the blob. Errors returned by be.Load()
// are passed on, except io.ErrUnexpectedEOF is silenced and nil returned
// instead, since it means this function is working properly.
func LoadAll(be Backend, h Handle, buf []byte) ([]byte, error) {
	fi, err := be.Stat(h)
	if err != nil {
		return nil, err
	}

	if fi.Size > int64(len(buf)) {
		buf = make([]byte, int(fi.Size))
	}

	n, err := be.Load(h, buf, 0)
	if errors.Cause(err) == io.ErrUnexpectedEOF {
		err = nil
	}
	buf = buf[:n]
	return buf, err
}
