package gcs

import (
	"bytes"
	"io"
	"io/ioutil"
	"strings"

	"google.golang.org/cloud/storage"
)

type fakeGCS struct {
	blobs map[string]*fakeRWC
}

func (s *fakeGCS) CopyObject(srcName, dstName string) error {
	if _, ok := s.blobs[srcName]; !ok {
		return storage.ErrObjectNotExist
	}
	s.blobs[dstName] = s.blobs[srcName]
	return nil
}

func (s *fakeGCS) DeleteObject(name string) error {
	if _, ok := s.blobs[name]; !ok {
		return storage.ErrObjectNotExist
	}
	delete(s.blobs, name)
	return nil
}

func (s *fakeGCS) StatObject(name string) error {
	if _, ok := s.blobs[name]; !ok {
		return storage.ErrObjectNotExist
	}
	return nil
}

func (s *fakeGCS) ListObjects(q *storage.Query) (*storage.Objects, error) {
	var names []*storage.Object
	for k := range s.blobs {
		if strings.HasPrefix(k, q.Prefix) {
			names = append(names, &storage.Object{Name: k})
		}
	}
	return &storage.Objects{
		Results: names,
	}, nil
}

func (s *fakeGCS) NewReader(name string) (io.ReadCloser, error) {
	if _, ok := s.blobs[name]; !ok {
		return nil, storage.ErrObjectNotExist
	}
	return ioutil.NopCloser(bytes.NewBuffer(s.blobs[name].Bytes())), nil
}

func (s *fakeGCS) NewWriter(name string) io.WriteCloser {
	b := &fakeRWC{
		bytes.Buffer{},
	}
	s.blobs[name] = b
	return b
}

func (s *fakeGCS) String() string {
	return "fakeGCS"
}

type fakeRWC struct {
	bytes.Buffer
}

func (rwc *fakeRWC) Close() error {
	return nil
}
