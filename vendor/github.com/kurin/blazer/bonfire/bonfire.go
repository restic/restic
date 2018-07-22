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

// Package bonfire implements the B2 service.
package bonfire

import (
	"crypto/sha1"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"sync"

	"github.com/kurin/blazer/internal/pyre"
)

type FS string

func (f FS) open(fp string) (io.WriteCloser, error) {
	if err := os.MkdirAll(filepath.Dir(fp), 0755); err != nil {
		return nil, err
	}
	return os.Create(fp)
}

func (f FS) PartWriter(id string, part int) (io.WriteCloser, error) {
	fp := filepath.Join(string(f), id, fmt.Sprintf("%d", part))
	return f.open(fp)
}

func (f FS) Writer(bucket, name, id string) (io.WriteCloser, error) {
	fp := filepath.Join(string(f), bucket, name, id)
	return f.open(fp)
}

func (f FS) Parts(id string) ([]string, error) {
	dir := filepath.Join(string(f), id)
	file, err := os.Open(dir)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	fs, err := file.Readdir(0)
	if err != nil {
		return nil, err
	}
	shas := make([]string, len(fs)-1)
	for _, fi := range fs {
		if fi.Name() == "info" {
			continue
		}
		i, err := strconv.ParseInt(fi.Name(), 10, 32)
		if err != nil {
			return nil, err
		}
		p, err := os.Open(filepath.Join(dir, fi.Name()))
		if err != nil {
			return nil, err
		}
		sha := sha1.New()
		if _, err := io.Copy(sha, p); err != nil {
			p.Close()
			return nil, err
		}
		p.Close()
		shas[int(i)-1] = fmt.Sprintf("%x", sha.Sum(nil))
	}
	return shas, nil
}

type fi struct {
	Name   string
	Bucket string
}

func (f FS) Start(bucketId, fileName, fileId string, bs []byte) error {
	w, err := f.open(filepath.Join(string(f), fileId, "info"))
	if err != nil {
		return err
	}
	if err := json.NewEncoder(w).Encode(fi{Name: fileName, Bucket: bucketId}); err != nil {
		w.Close()
		return err
	}
	return w.Close()
}

func (f FS) Finish(fileId string) error {
	r, err := os.Open(filepath.Join(string(f), fileId, "info"))
	if err != nil {
		return err
	}
	defer r.Close()
	var info fi
	if err := json.NewDecoder(r).Decode(&info); err != nil {
		return err
	}
	shas, err := f.Parts(fileId) // oh my god this is terrible
	if err != nil {
		return err
	}
	w, err := f.open(filepath.Join(string(f), info.Bucket, info.Name, fileId))
	if err != nil {
		return err
	}
	for i := 1; i <= len(shas); i++ {
		r, err := os.Open(filepath.Join(string(f), fileId, fmt.Sprintf("%d", i)))
		if err != nil {
			w.Close()
			return err
		}
		if _, err := io.Copy(w, r); err != nil {
			w.Close()
			r.Close()
			return err
		}
		r.Close()
	}
	if err := w.Close(); err != nil {
		return err
	}
	return os.RemoveAll(filepath.Join(string(f), fileId))
}

func (f FS) ObjectByName(bucket, name string) (pyre.DownloadableObject, error) {
	dir := filepath.Join(string(f), bucket, name)
	d, err := os.Open(dir)
	if err != nil {
		return nil, err
	}
	defer d.Close()
	fis, err := d.Readdir(0)
	if err != nil {
		return nil, err
	}
	sort.Slice(fis, func(i, j int) bool { return fis[i].ModTime().Before(fis[j].ModTime()) })
	o, err := os.Open(filepath.Join(dir, fis[0].Name()))
	if err != nil {
		return nil, err
	}
	return do{
		o:    o,
		size: fis[0].Size(),
	}, nil
}

type do struct {
	size int64
	o    *os.File
}

func (d do) Size() int64         { return d.size }
func (d do) Reader() io.ReaderAt { return d.o }
func (d do) Close() error        { return d.o.Close() }

func (f FS) Get(fileId string) ([]byte, error) { return nil, nil }

type Localhost int

func (l Localhost) String() string                               { return fmt.Sprintf("http://localhost:%d", l) }
func (l Localhost) UploadHost(id string) (string, error)         { return l.String(), nil }
func (Localhost) Authorize(string, string) (string, error)       { return "ok", nil }
func (Localhost) CheckCreds(string, string) error                { return nil }
func (l Localhost) APIRoot(string) string                        { return l.String() }
func (l Localhost) DownloadRoot(string) string                   { return l.String() }
func (Localhost) Sizes(string) (int32, int32)                    { return 1e5, 1 }
func (l Localhost) UploadPartHost(fileId string) (string, error) { return l.String(), nil }

type LocalBucket struct {
	Port int

	mux sync.Mutex
	b   map[string][]byte
	nti map[string]string
}

func (lb *LocalBucket) AddBucket(id, name string, bs []byte) error {
	lb.mux.Lock()
	defer lb.mux.Unlock()

	if lb.b == nil {
		lb.b = make(map[string][]byte)
	}

	if lb.nti == nil {
		lb.nti = make(map[string]string)
	}

	lb.b[id] = bs
	lb.nti[name] = id
	return nil
}

func (lb *LocalBucket) RemoveBucket(id string) error {
	lb.mux.Lock()
	defer lb.mux.Unlock()

	if lb.b == nil {
		lb.b = make(map[string][]byte)
	}

	delete(lb.b, id)
	return nil
}

func (lb *LocalBucket) UpdateBucket(id string, rev int, bs []byte) error {
	return errors.New("no")
}

func (lb *LocalBucket) ListBuckets(acct string) ([][]byte, error) {
	lb.mux.Lock()
	defer lb.mux.Unlock()

	var bss [][]byte
	for _, bs := range lb.b {
		bss = append(bss, bs)
	}
	return bss, nil
}

func (lb *LocalBucket) GetBucket(id string) ([]byte, error) {
	lb.mux.Lock()
	defer lb.mux.Unlock()

	bs, ok := lb.b[id]
	if !ok {
		return nil, errors.New("not found")
	}
	return bs, nil
}

func (lb *LocalBucket) GetBucketID(name string) (string, error) {
	lb.mux.Lock()
	defer lb.mux.Unlock()

	id, ok := lb.nti[name]
	if !ok {
		return "", errors.New("not found")
	}
	return id, nil
}
