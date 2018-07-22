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
)

const uploadFilePartPrefix = "/b2api/v1/b2_upload_part/"

type LargeFileManager interface {
	PartWriter(id string, part int) (io.WriteCloser, error)
}

type largeFileServer struct {
	fm LargeFileManager
}

type uploadPartRequest struct {
	ID   string `json:"fileId"`
	Part int    `json:"partNumber"`
	Size int64  `json:"contentLength"`
	Hash string `json:"contentSha1"`
}

func parseUploadPartHeaders(r *http.Request) (uploadPartRequest, error) {
	var ur uploadPartRequest
	ur.Hash = r.Header.Get("X-Bz-Content-Sha1")
	part, err := strconv.ParseInt(r.Header.Get("X-Bz-Part-Number"), 10, 64)
	if err != nil {
		return ur, err
	}
	ur.Part = int(part)
	size, err := strconv.ParseInt(r.Header.Get("Content-Length"), 10, 64)
	if err != nil {
		return ur, err
	}
	ur.Size = size
	ur.ID = strings.TrimPrefix(r.URL.Path, uploadFilePartPrefix)
	return ur, nil
}

func (fs *largeFileServer) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	req, err := parseUploadPartHeaders(r)
	if err != nil {
		http.Error(rw, err.Error(), 500)
		fmt.Println("oh no")
		return
	}
	w, err := fs.fm.PartWriter(req.ID, req.Part)
	if err != nil {
		http.Error(rw, err.Error(), 500)
		fmt.Println("oh no")
		return
	}
	if _, err := io.Copy(w, io.LimitReader(r.Body, req.Size)); err != nil {
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
	if err := json.NewEncoder(rw).Encode(req); err != nil {
		fmt.Println("oh no")
	}
}

func RegisterLargeFileManagerOnMux(f LargeFileManager, mux *http.ServeMux) {
	mux.Handle(uploadFilePartPrefix, &largeFileServer{fm: f})
}
