package s3

import (
	"fmt"
	"io"
)

// ContinuousReader implements an io.Reader on top of an io.ReaderAt, advancing
// an offset.
type ContinuousReader struct {
	R      io.ReaderAt
	Offset int64
}

func (c *ContinuousReader) Read(p []byte) (int, error) {
	fmt.Printf("ContinuousReader %p: ReadAt(offset %v)\n", c, c.Offset)
	n, err := c.R.ReadAt(p, c.Offset)
	fmt.Printf("ContinuousReader %p: len(p) = %v, n %v, err %v\n",
		c, len(p), n, err)
	fmt.Printf("  %02x\n", p[:n])
	c.Offset += int64(n)
	return n, err
}
