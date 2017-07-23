// Copyright 2017, Google
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package b2

import (
	"bytes"
	"crypto/sha1"
	"fmt"
	"hash"
	"io"
	"io/ioutil"
	"os"
	"sync"
)

type writeBuffer interface {
	io.Writer
	Len() int
	Reader() (io.ReadSeeker, error)
	Hash() string // sha1 or whatever it is
	Close() error
}

type memoryBuffer struct {
	buf *bytes.Buffer
	hsh hash.Hash
	w   io.Writer
	mux sync.Mutex
}

var bufpool *sync.Pool

func init() {
	bufpool = &sync.Pool{}
	bufpool.New = func() interface{} { return &bytes.Buffer{} }
}

func newMemoryBuffer() *memoryBuffer {
	mb := &memoryBuffer{
		hsh: sha1.New(),
	}
	mb.buf = bufpool.Get().(*bytes.Buffer)
	mb.w = io.MultiWriter(mb.hsh, mb.buf)
	return mb
}

type thing struct {
	rs io.ReadSeeker
	t  int
}

func (mb *memoryBuffer) Write(p []byte) (int, error)    { return mb.w.Write(p) }
func (mb *memoryBuffer) Len() int                       { return mb.buf.Len() }
func (mb *memoryBuffer) Reader() (io.ReadSeeker, error) { return bytes.NewReader(mb.buf.Bytes()), nil }
func (mb *memoryBuffer) Hash() string                   { return fmt.Sprintf("%x", mb.hsh.Sum(nil)) }

func (mb *memoryBuffer) Close() error {
	mb.mux.Lock()
	defer mb.mux.Unlock()
	if mb.buf == nil {
		return nil
	}
	mb.buf.Truncate(0)
	bufpool.Put(mb.buf)
	mb.buf = nil
	return nil
}

type fileBuffer struct {
	f   *os.File
	hsh hash.Hash
	w   io.Writer
	s   int
}

func newFileBuffer(loc string) (*fileBuffer, error) {
	f, err := ioutil.TempFile(loc, "blazer")
	if err != nil {
		return nil, err
	}
	fb := &fileBuffer{
		f:   f,
		hsh: sha1.New(),
	}
	fb.w = io.MultiWriter(fb.f, fb.hsh)
	return fb, nil
}

func (fb *fileBuffer) Write(p []byte) (int, error) {
	n, err := fb.w.Write(p)
	fb.s += n
	return n, err
}

func (fb *fileBuffer) Len() int     { return fb.s }
func (fb *fileBuffer) Hash() string { return fmt.Sprintf("%x", fb.hsh.Sum(nil)) }

func (fb *fileBuffer) Reader() (io.ReadSeeker, error) {
	if _, err := fb.f.Seek(0, 0); err != nil {
		return nil, err
	}
	return &fr{f: fb.f}, nil
}

func (fb *fileBuffer) Close() error {
	fb.f.Close()
	return os.Remove(fb.f.Name())
}

// wraps *os.File so that the http package doesn't see it as an io.Closer
type fr struct {
	f *os.File
}

func (r *fr) Read(p []byte) (int, error)         { return r.f.Read(p) }
func (r *fr) Seek(a int64, b int) (int64, error) { return r.f.Seek(a, b) }
