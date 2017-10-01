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

// Package consistent implements an experimental interface for using B2 as a
// coordination primitive.
package consistent

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"reflect"

	"github.com/kurin/blazer/b2"
)

const metaKey = "blazer-meta-key-no-touchie"

var (
	errUpdateConflict = errors.New("update conflict")
	errNotInGroup     = errors.New("not in group")
)

// NewGroup creates a new consistent Group for the given bucket.
func NewGroup(bucket *b2.Bucket, name string) *Group {
	return &Group{
		name: name,
		b:    bucket,
	}
}

// Group represents a collection of B2 objects that can be modified in a
// consistent way.  Objects in the same group contend with each other for
// updates, but there can only be so many (maximum of 10; fewer if there are
// other bucket attributes set) groups in a given bucket.
type Group struct {
	name string
	b    *b2.Bucket
	ba   *b2.BucketAttrs
}

// Operate calls f with the contents of the group object given by name, and
// updates that object with the output of f if f returns no error.  Operate
// guarantees that no other callers have modified the contents of name in the
// meantime (as long as all other callers are using this package).  It may call
// f any number of times and, as a result, the potential data transfer is
// unbounded.  Callers should have f fail after a given number of attempts if
// this is unacceptable.
//
// The io.Reader that f returns is guaranteed to be read until at least the
// first error.  Callers must ensure that this is sufficient for the reader to
// clean up after itself.
func (g *Group) OperateStream(ctx context.Context, name string, f func(io.Reader) (io.Reader, error)) error {
	for {
		r, err := g.NewReader(ctx, name)
		if err != nil && err != errNotInGroup {
			return err
		}
		out, err := f(r)
		r.Close()
		if err != nil {
			return err
		}
		defer io.Copy(ioutil.Discard, out) // ensure the reader is read
		w, err := g.NewWriter(ctx, r.Key, name)
		if err != nil {
			return err
		}
		if _, err := io.Copy(w, out); err != nil {
			return err
		}
		if err := w.Close(); err != nil {
			if err == errUpdateConflict {
				continue
			}
			return err
		}
		return nil
	}
}

// Operate uses OperateStream to act on byte slices.
func (g *Group) Operate(ctx context.Context, name string, f func([]byte) ([]byte, error)) error {
	return g.OperateStream(ctx, name, func(r io.Reader) (io.Reader, error) {
		b, err := ioutil.ReadAll(r)
		if b2.IsNotExist(err) {
			b = nil
			err = nil
		}
		if err != nil {
			return nil, err
		}
		bs, err := f(b)
		if err != nil {
			return nil, err
		}
		return bytes.NewReader(bs), nil
	})
}

// OperateJSON is a convenience function for transforming JSON data in B2 in a
// consistent way.  Callers should pass a function f which accepts a pointer to
// a struct of a given type and transforms it into another struct (ideally but
// not necessarily of the same type).  Callers should also pass an example
// struct, t, or a pointer to it, that is the same type.  t will not be
// altered.  If there is no existing file, f will be called with an pointer to
// an empty struct of type t.  Otherwise, it will be called with a pointer to a
// struct filled out with the given JSON.
func (g *Group) OperateJSON(ctx context.Context, name string, t interface{}, f func(interface{}) (interface{}, error)) error {
	jsonType := reflect.TypeOf(t)
	for jsonType.Kind() == reflect.Ptr {
		jsonType = jsonType.Elem()
	}
	return g.OperateStream(ctx, name, func(r io.Reader) (io.Reader, error) {
		in := reflect.New(jsonType).Interface()
		if err := json.NewDecoder(r).Decode(in); err != nil && err != io.EOF && !b2.IsNotExist(err) {
			return nil, err
		}
		out, err := f(in)
		if err != nil {
			return nil, err
		}
		pr, pw := io.Pipe()
		go func() { pw.CloseWithError(json.NewEncoder(pw).Encode(out)) }()
		return closeAfterReading{rc: pr}, nil
	})
}

// closeAfterReading closes the underlying reader on the first non-nil error
type closeAfterReading struct {
	rc io.ReadCloser
}

func (car closeAfterReading) Read(p []byte) (int, error) {
	n, err := car.rc.Read(p)
	if err != nil {
		car.rc.Close()
	}
	return n, err
}

// Writer is an io.ReadCloser.
type Writer struct {
	ctx    context.Context
	wc     io.WriteCloser
	name   string
	suffix string
	key    string
	g      *Group
}

// Write implements io.Write.
func (w Writer) Write(p []byte) (int, error) { return w.wc.Write(p) }

// Close writes any remaining data into B2 and updates the group to reflect the
// contents of the new object.  If the group object has been modified, Close()
// will fail.
func (w Writer) Close() error {
	if err := w.wc.Close(); err != nil {
		return err
	}
	// TODO: maybe see if you can cut down on calls to info()
	for {
		ci, err := w.g.info(w.ctx)
		if err != nil {
			// Replacement failed; delete the new version.
			w.g.b.Object(w.name + "/" + w.suffix).Delete(w.ctx)
			return err
		}
		old, ok := ci.Locations[w.name]
		if ok && old != w.key {
			w.g.b.Object(w.name + "/" + w.suffix).Delete(w.ctx)
			return errUpdateConflict
		}
		ci.Locations[w.name] = w.suffix
		if err := w.g.save(w.ctx, ci); err != nil {
			if err == errUpdateConflict {
				continue
			}
			w.g.b.Object(w.name + "/" + w.suffix).Delete(w.ctx)
			return err
		}
		// Replacement successful; delete the old version.
		w.g.b.Object(w.name + "/" + w.key).Delete(w.ctx)
		return nil
	}
}

// Reader is an io.ReadCloser.  Key must be passed to NewWriter.
type Reader struct {
	r   io.ReadCloser
	Key string
}

func (r Reader) Read(p []byte) (int, error) {
	if r.r == nil {
		return 0, io.EOF
	}
	return r.r.Read(p)
}

func (r Reader) Close() error {
	if r.r == nil {
		return nil
	}
	return r.r.Close()
}

// NewWriter creates a Writer and prepares it to be updated.  The key argument
// should come from the Key field of a Reader; if Writer.Close() returns with
// no error, then the underlying group object was successfully updated from the
// data available from the Reader with no intervening writes.  New objects can
// be created with an empty key.
func (g *Group) NewWriter(ctx context.Context, key, name string) (Writer, error) {
	suffix, err := random()
	if err != nil {
		return Writer{}, err
	}
	return Writer{
		ctx:    ctx,
		wc:     g.b.Object(name + "/" + suffix).NewWriter(ctx),
		name:   name,
		suffix: suffix,
		key:    key,
		g:      g,
	}, nil
}

// NewReader creates a Reader with the current version of the object, as well
// as that object's update key.
func (g *Group) NewReader(ctx context.Context, name string) (Reader, error) {
	ci, err := g.info(ctx)
	if err != nil {
		return Reader{}, err
	}
	suffix, ok := ci.Locations[name]
	if !ok {
		return Reader{}, errNotInGroup
	}
	return Reader{
		r:   g.b.Object(name + "/" + suffix).NewReader(ctx),
		Key: suffix,
	}, nil
}

func (g *Group) info(ctx context.Context) (*consistentInfo, error) {
	attrs, err := g.b.Attrs(ctx)
	if err != nil {
		return nil, err
	}
	g.ba = attrs
	imap := attrs.Info
	if imap == nil {
		return nil, nil
	}
	enc, ok := imap[metaKey+"-"+g.name]
	if !ok {
		return &consistentInfo{
			Version:   1,
			Locations: make(map[string]string),
		}, nil
	}
	b, err := base64.StdEncoding.DecodeString(enc)
	if err != nil {
		return nil, err
	}
	ci := &consistentInfo{}
	if err := json.Unmarshal(b, ci); err != nil {
		return nil, err
	}
	if ci.Locations == nil {
		ci.Locations = make(map[string]string)
	}
	return ci, nil
}

func (g *Group) save(ctx context.Context, ci *consistentInfo) error {
	ci.Serial++
	b, err := json.Marshal(ci)
	if err != nil {
		return err
	}
	s := base64.StdEncoding.EncodeToString(b)

	for {
		oldAI, err := g.info(ctx)
		if err != nil {
			return err
		}
		if oldAI.Serial != ci.Serial-1 {
			return errUpdateConflict
		}
		if g.ba.Info == nil {
			g.ba.Info = make(map[string]string)
		}
		g.ba.Info[metaKey+"-"+g.name] = s
		err = g.b.Update(ctx, g.ba)
		if err == nil {
			return nil
		}
		if !b2.IsUpdateConflict(err) {
			return err
		}
		// Bucket update conflict; try again.
	}
}

// List returns a list of all the group objects.
func (g *Group) List(ctx context.Context) ([]string, error) {
	ci, err := g.info(ctx)
	if err != nil {
		return nil, err
	}
	var l []string
	for name := range ci.Locations {
		l = append(l, name)
	}
	return l, nil
}

type consistentInfo struct {
	Version int

	// Serial is incremented for every version saved.  If we ensure that
	// current.Serial = 1 + previous.Serial, and that the bucket metadata is
	// updated cleanly, then we know that the version we saved is the direct
	// successor to the version we had.  If the bucket metadata doesn't update
	// cleanly, but the serial relation holds true for the new AI struct, then we
	// can retry without bothering the user.  However, if the serial relation no
	// longer holds true, it means someone else has updated AI and we have to ask
	// the user to redo everything they've done.
	//
	// However, it is still necessary for higher level constructs to confirm that
	// the serial number they expect is good.  The writer does this, for example,
	// but comparing the "key" of the file it is replacing.
	Serial    int
	Locations map[string]string
}

func random() (string, error) {
	b := make([]byte, 20)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", b), nil
}
