// Copyright 2016, Google
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
	"io"
	"sync"

	"github.com/kurin/blazer/internal/blog"

	"golang.org/x/net/context"
)

// Reader reads files from B2.
type Reader struct {
	// ConcurrentDownloads is the number of simultaneous downloads to pull from
	// B2.  Values greater than one will cause B2 to make multiple HTTP requests
	// for a given file, increasing available bandwidth at the cost of buffering
	// the downloads in memory.
	ConcurrentDownloads int

	// ChunkSize is the size to fetch per ConcurrentDownload.  The default is
	// 10MB.
	ChunkSize int

	ctx    context.Context
	cancel context.CancelFunc // cancels ctx
	o      *Object
	name   string
	offset int64 // the start of the file
	length int64 // the length to read, or -1
	size   int64 // the end of the file, in absolute terms
	csize  int   // chunk size
	read   int   // amount read
	chwid  int   // chunks written
	chrid  int   // chunks read
	chbuf  chan *bytes.Buffer
	init   sync.Once
	rmux   sync.Mutex // guards rcond
	rcond  *sync.Cond
	chunks map[int]*bytes.Buffer

	emux sync.RWMutex // guards err, believe it or not
	err  error

	smux sync.Mutex
	smap map[int]*meteredReader
}

// Close frees resources associated with the download.
func (r *Reader) Close() error {
	r.cancel()
	r.o.b.c.removeReader(r)
	return nil
}

func (r *Reader) setErr(err error) {
	r.emux.Lock()
	defer r.emux.Unlock()
	if r.err == nil {
		r.err = err
		r.cancel()
	}
}

func (r *Reader) setErrNoCancel(err error) {
	r.emux.Lock()
	defer r.emux.Unlock()
	if r.err == nil {
		r.err = err
	}
}

func (r *Reader) getErr() error {
	r.emux.RLock()
	defer r.emux.RUnlock()
	return r.err
}

func (r *Reader) thread() {
	go func() {
		for {
			var buf *bytes.Buffer
			select {
			case b, ok := <-r.chbuf:
				if !ok {
					return
				}
				buf = b
			case <-r.ctx.Done():
				return
			}
			r.rmux.Lock()
			chunkID := r.chwid
			r.chwid++
			r.rmux.Unlock()
			offset := int64(chunkID*r.csize) + r.offset
			size := int64(r.csize)
			if offset >= r.size {
				// Send an empty chunk.  This is necessary to prevent a deadlock when
				// this is the very first chunk.
				r.rmux.Lock()
				r.chunks[chunkID] = buf
				r.rmux.Unlock()
				r.rcond.Broadcast()
				return
			}
			if offset+size > r.size {
				size = r.size - offset
			}
		redo:
			fr, err := r.o.b.b.downloadFileByName(r.ctx, r.name, offset, size)
			if err != nil {
				r.setErr(err)
				r.rcond.Broadcast()
				return
			}
			mr := &meteredReader{r: &fakeSeeker{fr}, size: int(size)}
			r.smux.Lock()
			r.smap[chunkID] = mr
			r.smux.Unlock()
			i, err := copyContext(r.ctx, buf, mr)
			r.smux.Lock()
			r.smap[chunkID] = nil
			r.smux.Unlock()
			if i < size || err == io.ErrUnexpectedEOF {
				// Probably the network connection was closed early.  Retry.
				blog.V(1).Infof("b2 reader %d: got %dB of %dB; retrying", chunkID, i, size)
				buf.Reset()
				goto redo
			}
			if err != nil {
				r.setErr(err)
				r.rcond.Broadcast()
				return
			}
			r.rmux.Lock()
			r.chunks[chunkID] = buf
			r.rmux.Unlock()
			r.rcond.Broadcast()
		}
	}()
}

func (r *Reader) curChunk() (*bytes.Buffer, error) {
	ch := make(chan *bytes.Buffer)
	go func() {
		r.rmux.Lock()
		defer r.rmux.Unlock()
		for r.chunks[r.chrid] == nil && r.getErr() == nil && r.ctx.Err() == nil {
			r.rcond.Wait()
		}
		select {
		case ch <- r.chunks[r.chrid]:
		case <-r.ctx.Done():
			return
		}
	}()
	select {
	case buf := <-ch:
		return buf, r.getErr()
	case <-r.ctx.Done():
		if r.getErr() != nil {
			return nil, r.getErr()
		}
		return nil, r.ctx.Err()
	}
}

func (r *Reader) initFunc() {
	r.smux.Lock()
	r.smap = make(map[int]*meteredReader)
	r.smux.Unlock()
	r.o.b.c.addReader(r)
	if err := r.o.ensure(r.ctx); err != nil {
		r.setErr(err)
		return
	}
	r.size = r.o.f.size()
	if r.length >= 0 && r.offset+r.length < r.size {
		r.size = r.offset + r.length
	}
	if r.offset > r.size {
		r.offset = r.size
	}
	r.rcond = sync.NewCond(&r.rmux)
	cr := r.ConcurrentDownloads
	if cr < 1 {
		cr = 1
	}
	if r.ChunkSize < 1 {
		r.ChunkSize = 1e7
	}
	r.csize = r.ChunkSize
	r.chbuf = make(chan *bytes.Buffer, cr)
	for i := 0; i < cr; i++ {
		r.thread()
		r.chbuf <- &bytes.Buffer{}
	}
}

func (r *Reader) Read(p []byte) (int, error) {
	if err := r.getErr(); err != nil {
		return 0, err
	}
	// TODO: check the SHA1 hash here and verify it on Close.
	r.init.Do(r.initFunc)
	chunk, err := r.curChunk()
	if err != nil {
		r.setErrNoCancel(err)
		return 0, err
	}
	n, err := chunk.Read(p)
	r.read += n
	if err == io.EOF {
		if int64(r.read) >= r.size-r.offset {
			close(r.chbuf)
			r.setErrNoCancel(err)
			return n, err
		}
		r.chrid++
		chunk.Reset()
		r.chbuf <- chunk
		err = nil
	}
	r.setErrNoCancel(err)
	return n, err
}

func (r *Reader) status() *ReaderStatus {
	r.smux.Lock()
	defer r.smux.Unlock()

	rs := &ReaderStatus{
		Progress: make([]float64, len(r.smap)),
	}

	for i := 1; i <= len(r.smap); i++ {
		rs.Progress[i-1] = r.smap[i].done()
	}

	return rs
}

// copied from io.Copy, basically.
func copyContext(ctx context.Context, dst io.Writer, src io.Reader) (written int64, err error) {
	buf := make([]byte, 32*1024)
	for {
		if ctx.Err() != nil {
			err = ctx.Err()
			return
		}
		nr, er := src.Read(buf)
		if nr > 0 {
			nw, ew := dst.Write(buf[0:nr])
			if nw > 0 {
				written += int64(nw)
			}
			if ew != nil {
				err = ew
				break
			}
			if nr != nw {
				err = io.ErrShortWrite
				break
			}
		}
		if er == io.EOF {
			break
		}
		if er != nil {
			err = er
			break
		}
	}
	return written, err
}

// fakeSeeker exists so that we can wrap the http response body (an io.Reader
// but not an io.Seeker) into a meteredReader, which will allow us to keep tabs
// on how much of the chunk we've read so far.
type fakeSeeker struct {
	io.Reader
}

func (fs *fakeSeeker) Seek(int64, int) (int64, error) { return 0, nil }
