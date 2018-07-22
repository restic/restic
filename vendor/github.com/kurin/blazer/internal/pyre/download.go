// Copyright 2018, Google
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

package pyre

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

type DownloadableObject interface {
	Size() int64
	Reader() io.ReaderAt
	io.Closer
}

type DownloadManager interface {
	ObjectByName(bucketID, name string) (DownloadableObject, error)
	GetBucketID(bucket string) (string, error)
	GetBucket(id string) ([]byte, error)
}

type downloadServer struct {
	dm DownloadManager
}

type downloadRequest struct {
	off, n int64
}

func parseDownloadHeaders(r *http.Request) (*downloadRequest, error) {
	rang := r.Header.Get("Range")
	if rang == "" {
		return &downloadRequest{}, nil
	}
	if !strings.HasPrefix(rang, "bytes=") {
		return nil, fmt.Errorf("unknown range format: %q", rang)
	}
	rang = strings.TrimPrefix(rang, "bytes=")
	if !strings.Contains(rang, "-") {
		return nil, fmt.Errorf("unknown range format: %q", rang)
	}
	parts := strings.Split(rang, "-")
	off, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return nil, err
	}
	end, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return nil, err
	}
	return &downloadRequest{
		off: off,
		n:   (end + 1) - off,
	}, nil
}

func (fs *downloadServer) serveWholeObject(rw http.ResponseWriter, obj DownloadableObject) {
	rw.Header().Set("Content-Length", fmt.Sprintf("%d", obj.Size()))
	sr := io.NewSectionReader(obj.Reader(), 0, obj.Size())
	if _, err := io.Copy(rw, sr); err != nil {
		http.Error(rw, err.Error(), 503)
		fmt.Println("no reader", err)
	}
}

func (fs *downloadServer) servePartialObject(rw http.ResponseWriter, obj DownloadableObject, off, len int64) {
	if off >= obj.Size() {
		http.Error(rw, "hell naw", 416)
		fmt.Printf("range not good (%d-%d for %d)\n", off, len, obj.Size())
		return
	}
	if off+len > obj.Size() {
		len = obj.Size() - off
	}
	sr := io.NewSectionReader(obj.Reader(), off, len)
	rw.Header().Set("Content-Length", fmt.Sprintf("%d", len))
	rw.WriteHeader(206) // this goes after headers are set
	if _, err := io.Copy(rw, sr); err != nil {
		fmt.Println("bad read:", err)
	}
}

func (fs *downloadServer) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	req, err := parseDownloadHeaders(r)
	if err != nil {
		http.Error(rw, err.Error(), 503)
		fmt.Println("weird header")
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/")
	parts := strings.Split(path, "/")
	if len(parts) < 3 {
		http.Error(rw, err.Error(), 404)
		fmt.Println("weird file")
		return
	}
	bucket := parts[1]
	bid, err := fs.dm.GetBucketID(bucket)
	if err != nil {
		http.Error(rw, err.Error(), 503)
		fmt.Println("no bucket:", err)
		return
	}
	file := strings.Join(parts[2:], "/")
	obj, err := fs.dm.ObjectByName(bid, file)
	if err != nil {
		http.Error(rw, err.Error(), 503)
		fmt.Println("no reader", err)
		return
	}
	defer obj.Close()
	if req.off == 0 && req.n == 0 {
		fs.serveWholeObject(rw, obj)
		return
	}
	fs.servePartialObject(rw, obj, req.off, req.n)
}

func RegisterDownloadManagerOnMux(d DownloadManager, mux *http.ServeMux) {
	mux.Handle("/file/", &downloadServer{dm: d})
}
