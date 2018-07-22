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
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/kurin/blazer/internal/b2types"
)

const uploadFilePrefix = "/b2api/v1/b2_upload_file/"

type SimpleFileManager interface {
	Writer(bucket, name, id string) (io.WriteCloser, error)
}

type simpleFileServer struct {
	fm SimpleFileManager
}

type uploadRequest struct {
	name        string
	contentType string
	size        int64
	sha1        string
	bucket      string
	info        map[string]string
}

func parseUploadHeaders(r *http.Request) (*uploadRequest, error) {
	ur := &uploadRequest{info: make(map[string]string)}
	ur.name = r.Header.Get("X-Bz-File-Name")
	ur.contentType = r.Header.Get("Content-Type")
	ur.sha1 = r.Header.Get("X-Bz-Content-Sha1")
	size, err := strconv.ParseInt(r.Header.Get("Content-Length"), 10, 64)
	if err != nil {
		return nil, err
	}
	ur.size = size
	for k := range r.Header {
		if !strings.HasPrefix("X-Bz-Info-", k) {
			continue
		}
		name := strings.TrimPrefix("X-Bz-Info-", k)
		ur.info[name] = r.Header.Get(k)
	}
	ur.bucket = strings.TrimPrefix(r.URL.Path, uploadFilePrefix)
	return ur, nil
}

func (fs *simpleFileServer) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	req, err := parseUploadHeaders(r)
	if err != nil {
		http.Error(rw, err.Error(), 500)
		fmt.Println("oh no")
		return
	}
	id := uuid.New().String()
	w, err := fs.fm.Writer(req.bucket, req.name, id)
	if err != nil {
		http.Error(rw, err.Error(), 500)
		fmt.Println("oh no")
		return
	}
	if _, err := io.Copy(w, io.LimitReader(r.Body, req.size)); err != nil {
		w.Close()
		http.Error(rw, err.Error(), 500)
		fmt.Println("oh no")
		return
	}
	if err := w.Close(); err != nil {
		http.Error(rw, err.Error(), 500)
		fmt.Println("oh no")
		return
	}
	resp := &b2types.UploadFileResponse{
		FileID:   id,
		Name:     req.name,
		SHA1:     req.sha1,
		BucketID: req.bucket,
	}
	if err := json.NewEncoder(rw).Encode(resp); err != nil {
		http.Error(rw, err.Error(), 500)
		fmt.Println("oh no")
		return
	}
}

func RegisterSimpleFileManagerOnMux(f SimpleFileManager, mux *http.ServeMux) {
	mux.Handle(uploadFilePrefix, &simpleFileServer{fm: f})
}
