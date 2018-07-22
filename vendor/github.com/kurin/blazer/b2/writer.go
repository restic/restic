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
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kurin/blazer/internal/blog"
)

// Writer writes data into Backblaze.  It automatically switches to the large
// file API if the file exceeds ChunkSize bytes.  Due to that and other
// Backblaze API details, there is a large buffer.
//
// Changes to public Writer attributes must be made before the first call to
// Write.
type Writer struct {
	// ConcurrentUploads is number of different threads sending data concurrently
	// to Backblaze for large files.  This can increase performance greatly, as
	// each thread will hit a different endpoint.  However, there is a ChunkSize
	// buffer for each thread.  Values less than 1 are equivalent to 1.
	ConcurrentUploads int

	// Resume an upload.  If true, and the upload is a large file, and a file of
	// the same name was started but not finished, then assume that we are
	// resuming that file, and don't upload duplicate chunks.
	Resume bool

	// ChunkSize is the size, in bytes, of each individual part, when writing
	// large files, and also when determining whether to upload a file normally
	// or when to split it into parts.  The default is 100M (1e8)  The minimum is
	// 5M (5e6); values less than this are not an error, but will fail.  The
	// maximum is 5GB (5e9).
	ChunkSize int

	// UseFileBuffer controls whether to use an in-memory buffer (the default) or
	// scratch space on the file system.  If this is true, b2 will save chunks in
	// FileBufferDir.
	UseFileBuffer bool

	// FileBufferDir specifies the directory where scratch files are kept.  If
	// blank, os.TempDir() is used.
	FileBufferDir string

	contentType string
	info        map[string]string

	csize       int
	ctx         context.Context
	cancel      context.CancelFunc
	ready       chan chunk
	wg          sync.WaitGroup
	start       sync.Once
	once        sync.Once
	done        sync.Once
	file        beLargeFileInterface
	seen        map[int]string
	everStarted bool
	newBuffer   func() (writeBuffer, error)

	o    *Object
	name string

	cidx int
	w    writeBuffer

	emux sync.RWMutex
	err  error

	smux sync.RWMutex
	smap map[int]*meteredReader
}

type chunk struct {
	id  int
	buf writeBuffer
}

func (w *Writer) setErr(err error) {
	if err == nil || err == io.EOF {
		return
	}
	w.emux.Lock()
	defer w.emux.Unlock()
	if w.err == nil {
		blog.V(1).Infof("error writing %s: %v", w.name, err)
		w.err = err
		w.cancel()
	}
}

func (w *Writer) getErr() error {
	w.emux.RLock()
	defer w.emux.RUnlock()
	return w.err
}

func (w *Writer) registerChunk(id int, r *meteredReader) {
	w.smux.Lock()
	w.smap[id] = r
	w.smux.Unlock()
}

func (w *Writer) completeChunk(id int) {
	w.smux.Lock()
	w.smap[id] = nil
	w.smux.Unlock()
}

var gid int32

func (w *Writer) thread() {
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		id := atomic.AddInt32(&gid, 1)
		fc, err := w.file.getUploadPartURL(w.ctx)
		if err != nil {
			w.setErr(err)
			return
		}
		for {
			chunk, ok := <-w.ready
			if !ok {
				return
			}
			if sha, ok := w.seen[chunk.id]; ok {
				if sha != chunk.buf.Hash() {
					w.setErr(errors.New("resumable upload was requested, but chunks don't match"))
					return
				}
				chunk.buf.Close()
				w.completeChunk(chunk.id)
				blog.V(2).Infof("skipping chunk %d", chunk.id)
				continue
			}
			blog.V(2).Infof("thread %d handling chunk %d", id, chunk.id)
			r, err := chunk.buf.Reader()
			if err != nil {
				w.setErr(err)
				return
			}
			mr := &meteredReader{r: r, size: chunk.buf.Len()}
			w.registerChunk(chunk.id, mr)
			sleep := time.Millisecond * 15
		redo:
			n, err := fc.uploadPart(w.ctx, mr, chunk.buf.Hash(), chunk.buf.Len(), chunk.id)
			if n != chunk.buf.Len() || err != nil {
				if w.o.b.r.reupload(err) {
					time.Sleep(sleep)
					sleep *= 2
					if sleep > time.Second*15 {
						sleep = time.Second * 15
					}
					blog.V(1).Infof("b2 writer: wrote %d of %d: error: %v; retrying", n, chunk.buf.Len(), err)
					f, err := w.file.getUploadPartURL(w.ctx)
					if err != nil {
						w.setErr(err)
						w.completeChunk(chunk.id)
						chunk.buf.Close() // TODO: log error
						return
					}
					fc = f
					goto redo
				}
				w.setErr(err)
				w.completeChunk(chunk.id)
				chunk.buf.Close() // TODO: log error
				return
			}
			w.completeChunk(chunk.id)
			chunk.buf.Close() // TODO: log error
			blog.V(2).Infof("chunk %d handled", chunk.id)
		}
	}()
}

func (w *Writer) init() {
	w.start.Do(func() {
		w.everStarted = true
		w.smux.Lock()
		w.smap = make(map[int]*meteredReader)
		w.smux.Unlock()
		w.o.b.c.addWriter(w)
		w.csize = w.ChunkSize
		if w.csize == 0 {
			w.csize = 1e8
		}
		if w.newBuffer == nil {
			w.newBuffer = func() (writeBuffer, error) { return newMemoryBuffer(), nil }
			if w.UseFileBuffer {
				w.newBuffer = func() (writeBuffer, error) { return newFileBuffer(w.FileBufferDir) }
			}
		}
		v, err := w.newBuffer()
		if err != nil {
			w.setErr(err)
			return
		}
		w.w = v
	})
}

// Write satisfies the io.Writer interface.
func (w *Writer) Write(p []byte) (int, error) {
	w.init()
	if err := w.getErr(); err != nil {
		return 0, err
	}
	left := w.csize - w.w.Len()
	if len(p) < left {
		return w.w.Write(p)
	}
	i, err := w.w.Write(p[:left])
	if err != nil {
		w.setErr(err)
		return i, err
	}
	if err := w.sendChunk(); err != nil {
		w.setErr(err)
		return i, w.getErr()
	}
	k, err := w.Write(p[left:])
	if err != nil {
		w.setErr(err)
	}
	return i + k, err
}

func (w *Writer) getUploadURL(ctx context.Context) (beURLInterface, error) {
	u := w.o.b.urlPool.get()
	if u == nil {
		return w.o.b.b.getUploadURL(w.ctx)
	}

	return u, nil
}

func (w *Writer) simpleWriteFile() error {
	ue, err := w.getUploadURL(w.ctx)
	if err != nil {
		return err
	}
	// This defer needs to be in a func() so that we put whatever the value of ue
	// is at function exit.
	defer func() { w.o.b.urlPool.put(ue) }()
	sha1 := w.w.Hash()
	ctype := w.contentType
	if ctype == "" {
		ctype = "application/octet-stream"
	}
	r, err := w.w.Reader()
	if err != nil {
		return err
	}
	mr := &meteredReader{r: r, size: w.w.Len()}
	w.registerChunk(1, mr)
	defer w.completeChunk(1)
redo:
	f, err := ue.uploadFile(w.ctx, mr, int(w.w.Len()), w.name, ctype, sha1, w.info)
	if err != nil {
		if w.o.b.r.reupload(err) {
			blog.V(2).Infof("b2 writer: %v; retrying", err)
			u, err := w.o.b.b.getUploadURL(w.ctx)
			if err != nil {
				return err
			}
			ue = u
			goto redo
		}
		return err
	}
	w.o.f = f
	return nil
}

func (w *Writer) getLargeFile() (beLargeFileInterface, error) {
	if !w.Resume {
		ctype := w.contentType
		if ctype == "" {
			ctype = "application/octet-stream"
		}
		return w.o.b.b.startLargeFile(w.ctx, w.name, ctype, w.info)
	}
	next := 1
	seen := make(map[int]string)
	var size int64
	var fi beFileInterface
	for {
		cur := &Cursor{name: w.name}
		objs, _, err := w.o.b.ListObjects(w.ctx, 1, cur)
		if err != nil {
			return nil, err
		}
		if len(objs) < 1 || objs[0].name != w.name {
			w.Resume = false
			return w.getLargeFile()
		}
		fi = objs[0].f
		parts, n, err := fi.listParts(w.ctx, next, 100)
		if err != nil {
			return nil, err
		}
		next = n
		for _, p := range parts {
			seen[p.number()] = p.sha1()
			size += p.size()
		}
		if len(parts) == 0 {
			break
		}
		if next == 0 {
			break
		}
	}
	w.seen = make(map[int]string) // copy the map
	for id, sha := range seen {
		w.seen[id] = sha
	}
	return fi.compileParts(size, seen), nil
}

func (w *Writer) sendChunk() error {
	var err error
	w.once.Do(func() {
		lf, e := w.getLargeFile()
		if e != nil {
			err = e
			return
		}
		w.file = lf
		w.ready = make(chan chunk)
		if w.ConcurrentUploads < 1 {
			w.ConcurrentUploads = 1
		}
		for i := 0; i < w.ConcurrentUploads; i++ {
			w.thread()
		}
	})
	if err != nil {
		return err
	}
	select {
	case w.ready <- chunk{
		id:  w.cidx + 1,
		buf: w.w,
	}:
	case <-w.ctx.Done():
		return w.ctx.Err()
	}
	w.cidx++
	v, err := w.newBuffer()
	if err != nil {
		return err
	}
	w.w = v
	return nil
}

// ReadFrom reads all of r into w, returning the first error or no error if r
// returns io.EOF.  If r is also an io.Seeker, ReadFrom will stream r directly
// over the wire instead of buffering it locally.  This reduces memory usage.
//
// Do not issue multiple calls to ReadFrom, or mix ReadFrom and Write.  If you
// have multiple readers you want to concatenate into the same B2 object, use
// an io.MultiReader.
//
// Note that io.Copy will automatically choose to use ReadFrom.
//
// ReadFrom currently doesn't handle w.Resume; if w.Resume is true, ReadFrom
// will act as if r is not an io.Seeker.
func (w *Writer) ReadFrom(r io.Reader) (int64, error) {
	rs, ok := r.(io.ReadSeeker)
	if !ok || w.Resume {
		return copyContext(w.ctx, w, r)
	}
	blog.V(2).Info("streaming without buffer")
	size, err := rs.Seek(0, io.SeekEnd)
	if err != nil {
		return 0, err
	}
	var ra io.ReaderAt
	if rat, ok := r.(io.ReaderAt); ok {
		ra = rat
	} else {
		ra = enReaderAt(rs)
	}
	var offset int64
	var wrote int64
	w.newBuffer = func() (writeBuffer, error) {
		left := size - offset
		if left <= 0 {
			// We're done sending real chunks; send empty chunks from now on so that
			// Close() works.
			w.newBuffer = func() (writeBuffer, error) { return newMemoryBuffer(), nil }
			w.w = newMemoryBuffer()
			return nil, io.EOF
		}
		csize := int64(w.csize)
		if left < csize {
			csize = left
		}
		nb := newNonBuffer(ra, offset, csize)
		wrote += csize // TODO: this is kind of a total lie
		offset += csize
		return nb, nil
	}
	w.init()
	if size < int64(w.csize) {
		// the magic happens on w.Close()
		return size, nil
	}
	for {
		if err := w.sendChunk(); err != nil {
			if err != io.EOF {
				return wrote, err
			}
			return wrote, nil
		}
	}
}

// Close satisfies the io.Closer interface.  It is critical to check the return
// value of Close for all writers.
func (w *Writer) Close() error {
	w.done.Do(func() {
		if !w.everStarted {
			return
		}
		defer w.o.b.c.removeWriter(w)
		defer func() {
			if err := w.w.Close(); err != nil {
				// this is non-fatal, but alarming
				blog.V(1).Infof("close %s: %v", w.name, err)
			}
		}()
		if w.cidx == 0 {
			w.setErr(w.simpleWriteFile())
			return
		}
		if w.w.Len() > 0 {
			if err := w.sendChunk(); err != nil {
				w.setErr(err)
				return
			}
		}
		close(w.ready)
		w.wg.Wait()
		f, err := w.file.finishLargeFile(w.ctx)
		if err != nil {
			w.setErr(err)
			return
		}
		w.o.f = f
	})
	return w.getErr()
}

// WithAttrs sets the writable attributes of the resulting file to given
// values.  WithAttrs must be called before the first call to Write.
//
// DEPRECATED: Use WithAttrsOption instead.
func (w *Writer) WithAttrs(attrs *Attrs) *Writer {
	w.contentType = attrs.ContentType
	w.info = make(map[string]string)
	for k, v := range attrs.Info {
		w.info[k] = v
	}
	if len(w.info) < 10 && attrs.SHA1 != "" {
		w.info["large_file_sha1"] = attrs.SHA1
	}
	if len(w.info) < 10 && !attrs.LastModified.IsZero() {
		w.info["src_last_modified_millis"] = fmt.Sprintf("%d", attrs.LastModified.UnixNano()/1e6)
	}
	return w
}

// A WriterOption sets Writer-specific behavior.
type WriterOption func(*Writer)

// WithAttrs attaches the given Attrs to the writer.
func WithAttrsOption(attrs *Attrs) WriterOption {
	return func(w *Writer) {
		w.WithAttrs(attrs)
	}
}

// DefaultWriterOptions returns a ClientOption that will apply the given
// WriterOptions to every Writer.  These options can be overridden by passing
// new options to NewWriter.
func DefaultWriterOptions(opts ...WriterOption) ClientOption {
	return func(c *clientOptions) {
		c.writerOpts = opts
	}
}

func (w *Writer) status() *WriterStatus {
	w.smux.RLock()
	defer w.smux.RUnlock()

	ws := &WriterStatus{
		Progress: make([]float64, len(w.smap)),
	}

	for i := 1; i <= len(w.smap); i++ {
		ws.Progress[i-1] = w.smap[i].done()
	}

	return ws
}

type meteredReader struct {
	read int64
	size int
	r    readResetter
	mux  sync.Mutex
}

func (mr *meteredReader) Read(p []byte) (int, error) {
	mr.mux.Lock()
	defer mr.mux.Unlock()
	n, err := mr.r.Read(p)
	mr.read += int64(n)
	return n, err
}

func (mr *meteredReader) Reset() error {
	mr.mux.Lock()
	defer mr.mux.Unlock()
	mr.read = 0
	return mr.r.Reset()
}

func (mr *meteredReader) done() float64 {
	if mr == nil {
		return 1
	}
	read := float64(atomic.LoadInt64(&mr.read))
	return read / float64(mr.size)
}
