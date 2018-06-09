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

package b2

import (
	"context"
	"io"
	"sync"
)

// List returns an iterator for selecting objects in a bucket.  The default
// behavior, with no options, is to list all currently un-hidden objects.
func (b *Bucket) List(ctx context.Context, opts ...ListOption) *ObjectIterator {
	o := &ObjectIterator{
		bucket: b,
		ctx:    ctx,
	}
	for _, opt := range opts {
		opt(&o.opts)
	}
	return o
}

// ObjectIterator abtracts away the tricky bits of iterating over a bucket's
// contents.
//
// It is intended to be called in a loop:
//  for iter.Next() {
//    obj := iter.Object()
//    // act on obj
//  }
//  if err := iter.Err(); err != nil {
//    // handle err
//  }
type ObjectIterator struct {
	bucket *Bucket
	ctx    context.Context
	final  bool
	err    error
	idx    int
	c      *Cursor
	opts   objectIteratorOptions
	objs   []*Object
	init   sync.Once
	l      lister
	count  int
}

type lister func(context.Context, int, *Cursor) ([]*Object, *Cursor, error)

func (o *ObjectIterator) page(ctx context.Context) error {
	if o.opts.locker != nil {
		o.opts.locker.Lock()
		defer o.opts.locker.Unlock()
	}
	objs, c, err := o.l(ctx, o.count, o.c)
	if err != nil && err != io.EOF {
		if bNotExist.MatchString(err.Error()) {
			return b2err{
				err:         err,
				notFoundErr: true,
			}
		}
		return err
	}
	o.c = c
	o.objs = objs
	o.idx = 0
	if err == io.EOF {
		o.final = true
	}
	return nil
}

// Next advances the iterator to the next object.  It should be called before
// any calls to Object().  If Next returns true, then the next call to Object()
// will be valid.  Once Next returns false, it is important to check the return
// value of Err().
func (o *ObjectIterator) Next() bool {
	o.init.Do(func() {
		o.count = o.opts.pageSize
		if o.count < 0 || o.count > 1000 {
			o.count = 1000
		}
		switch {
		case o.opts.unfinished:
			o.l = o.bucket.ListUnfinishedLargeFiles
			if o.count > 100 {
				o.count = 100
			}
		case o.opts.hidden:
			o.l = o.bucket.ListObjects
		default:
			o.l = o.bucket.ListCurrentObjects
		}
		o.c = &Cursor{
			Prefix:    o.opts.prefix,
			Delimiter: o.opts.delimiter,
		}
	})
	if o.err != nil {
		return false
	}
	if o.ctx.Err() != nil {
		o.err = o.ctx.Err()
		return false
	}
	if o.idx >= len(o.objs) {
		if o.final {
			o.err = io.EOF
			return false
		}
		if err := o.page(o.ctx); err != nil {
			o.err = err
			return false
		}
		return o.Next()
	}
	o.idx++
	return true
}

// Object returns the current object.
func (o *ObjectIterator) Object() *Object {
	return o.objs[o.idx-1]
}

// Err returns the current error or nil.  If Next() returns false and Err() is
// nil, then all objects have been seen.
func (o *ObjectIterator) Err() error {
	if o.err == io.EOF {
		return nil
	}
	return o.err
}

type objectIteratorOptions struct {
	hidden     bool
	unfinished bool
	prefix     string
	delimiter  string
	pageSize   int
	locker     sync.Locker
}

// A ListOption alters the default behavor of List.
type ListOption func(*objectIteratorOptions)

// ListHidden will include hidden objects in the output.
func ListHidden() ListOption {
	return func(o *objectIteratorOptions) {
		o.hidden = true
	}
}

// ListUnfinished will list unfinished large file operations instead of
// existing objects.
func ListUnfinished() ListOption {
	return func(o *objectIteratorOptions) {
		o.unfinished = true
	}
}

// ListPrefix will restrict the output to objects whose names begin with
// prefix.
func ListPrefix(pfx string) ListOption {
	return func(o *objectIteratorOptions) {
		o.prefix = pfx
	}
}

// ListDelimiter denotes the path separator.  If set, object listings will be
// truncated at this character.
//
// For example, if the bucket contains objects foo/bar, foo/baz, and foo,
// then a delimiter of "/" will cause the listing to return "foo" and "foo/".
// Otherwise, the listing would have returned all object names.
//
// Note that objects returned that end in the delimiter may not be actual
// objects, e.g. you cannot read from (or write to, or delete) an object
// "foo/", both because no actual object exists and because B2 disallows object
// names that end with "/".  If you want to ensure that all objects returned
// are actual objects, leave this unset.
func ListDelimiter(delimiter string) ListOption {
	return func(o *objectIteratorOptions) {
		o.delimiter = delimiter
	}
}

// ListPageSize configures the iterator to request the given number of objects
// per network round-trip.  The default (and maximum) is 1000 objects, except
// for unfinished large files, which is 100.
func ListPageSize(count int) ListOption {
	return func(o *objectIteratorOptions) {
		o.pageSize = count
	}
}

// ListLocker passes the iterator a lock which will be held during network
// round-trips.
func ListLocker(l sync.Locker) ListOption {
	return func(o *objectIteratorOptions) {
		o.locker = l
	}
}
