package fs

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"sync"
	"time"
)

// S3MemClient is an in-memory S3Client for tests.
type S3MemClient struct {
	mu      sync.Mutex
	objects map[string]S3Object
	content map[string][]byte

	Lists, Stats, Gets int

	// BeforeGet is called at the start of Get if set.
	BeforeGet func(key string)
	// BeforeList is called at the start of List if set.
	BeforeList func(prefix string)
}

// statically ensure that S3MemClient implements S3Client.
var _ S3Client = &S3MemClient{}

// NewS3MemClient returns an S3MemClient containing objects.
func NewS3MemClient(objects map[string][]byte, modTime time.Time) *S3MemClient {
	c := &S3MemClient{
		objects: make(map[string]S3Object),
		content: make(map[string][]byte),
	}
	for key, buf := range objects {
		c.Put(key, buf, modTime)
	}
	return c
}

// Put stores an object.
func (c *S3MemClient) Put(key string, buf []byte, modTime time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	sum := md5.Sum(buf)
	c.objects[key] = S3Object{
		Key:          key,
		Size:         int64(len(buf)),
		LastModified: modTime,
		ETag:         hex.EncodeToString(sum[:]),
	}
	c.content[key] = bytes.Clone(buf)
}

// Delete removes an object.
func (c *S3MemClient) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.objects, key)
	delete(c.content, key)
}

func (c *S3MemClient) List(_ context.Context, prefix string, fn func(S3Object) error) error {
	if c.BeforeList != nil {
		c.BeforeList(prefix)
	}
	c.mu.Lock()
	c.Lists++
	keys := make([]string, 0, len(c.objects))
	for key := range c.objects {
		keys = append(keys, key)
	}
	slices.Sort(keys)

	var result []S3Object
	lastPrefix := ""
	for _, key := range keys {
		rest, ok := strings.CutPrefix(key, prefix)
		if !ok {
			continue
		}
		if i := strings.Index(rest, "/"); i >= 0 {
			p := prefix + rest[:i+1]
			if p != lastPrefix {
				result = append(result, S3Object{Key: p})
				lastPrefix = p
			}
			continue
		}
		result = append(result, c.objects[key])
	}
	c.mu.Unlock()

	for _, obj := range result {
		if err := fn(obj); err != nil {
			return err
		}
	}
	return nil
}

func (c *S3MemClient) Stat(_ context.Context, key string) (S3Object, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Stats++
	obj, ok := c.objects[key]
	if !ok {
		return S3Object{}, fmt.Errorf("%w: %q", os.ErrNotExist, key)
	}
	return obj, nil
}

func (c *S3MemClient) Get(_ context.Context, key string) (io.ReadCloser, S3Object, error) {
	if c.BeforeGet != nil {
		c.BeforeGet(key)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Gets++
	obj, ok := c.objects[key]
	if !ok {
		return nil, S3Object{}, fmt.Errorf("%w: %q", os.ErrNotExist, key)
	}
	return io.NopCloser(bytes.NewReader(c.content[key])), obj, nil
}
