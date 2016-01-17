package s3

import "io"

// ContinuousReader implements an io.Reader on top of an io.ReaderAt, advancing
// an offset.
type ContinuousReader struct {
	R      io.ReaderAt
	Offset int64
}

func (c *ContinuousReader) Read(p []byte) (int, error) {
	n, err := c.R.ReadAt(p, c.Offset)
	c.Offset += int64(n)
	return n, err
}
