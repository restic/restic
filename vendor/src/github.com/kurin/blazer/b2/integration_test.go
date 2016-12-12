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
	"crypto/sha1"
	"fmt"
	"io"
	"net/http"
	"os"
	"reflect"
	"testing"
	"time"

	"golang.org/x/net/context"
)

const (
	apiID  = "B2_ACCOUNT_ID"
	apiKey = "B2_SECRET_KEY"
)

func TestReadWriteLive(t *testing.T) {
	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	bucket, done := startLiveTest(ctx, t)
	defer done()

	sobj, wsha, err := writeFile(ctx, bucket, smallFileName, 1e6-42, 1e8)
	if err != nil {
		t.Fatal(err)
	}

	lobj, wshaL, err := writeFile(ctx, bucket, largeFileName, 1e8+5e7, 1e8)
	if err != nil {
		t.Fatal(err)
	}

	if err := readFile(ctx, lobj, wshaL, 1e7, 10); err != nil {
		t.Error(err)
	}

	if err := readFile(ctx, sobj, wsha, 1e5, 10); err != nil {
		t.Error(err)
	}

	var cur *Cursor
	for {
		objs, c, err := bucket.ListObjects(ctx, 100, cur)
		if err != nil && err != io.EOF {
			t.Fatal(err)
		}
		for _, o := range objs {
			if err := o.Delete(ctx); err != nil {
				t.Error(err)
			}
		}
		if err == io.EOF {
			break
		}
		cur = c
	}
}

func TestHideShowLive(t *testing.T) {
	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	bucket, done := startLiveTest(ctx, t)
	defer done()

	// write a file
	obj, _, err := writeFile(ctx, bucket, smallFileName, 1e6+42, 1e8)
	if err != nil {
		t.Fatal(err)
	}

	got, err := countObjects(ctx, bucket.ListCurrentObjects)
	if err != nil {
		t.Error(err)
	}
	if got != 1 {
		t.Fatalf("got %d objects, wanted 1", got)
	}

	// hide the file
	if err := obj.Hide(ctx); err != nil {
		t.Fatal(err)
	}

	got, err = countObjects(ctx, bucket.ListCurrentObjects)
	if err != nil {
		t.Error(err)
	}
	if got != 0 {
		t.Fatalf("got %d objects, wanted 0", got)
	}

	// unhide the file
	if err := bucket.Reveal(ctx, smallFileName); err != nil {
		t.Fatal(err)
	}

	// count see the object again
	got, err = countObjects(ctx, bucket.ListCurrentObjects)
	if err != nil {
		t.Error(err)
	}
	if got != 1 {
		t.Fatalf("got %d objects, wanted 1", got)
	}
}

func TestResumeWriter(t *testing.T) {
	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	bucket, _ := startLiveTest(ctx, t)

	w := bucket.Object("foo").NewWriter(ctx)
	r := io.LimitReader(zReader{}, 3e8)
	go func() {
		// Cancel the context after the first chunk has been written.
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		defer cancel()
		for range ticker.C {
			if w.cidx > 1 {
				return
			}
		}
	}()
	if _, err := io.Copy(w, r); err != context.Canceled {
		t.Fatalf("io.Copy should have resulted in a canceled context")
	}

	ctx2 := context.Background()
	ctx2, cancel2 := context.WithTimeout(ctx2, 10*time.Minute)
	defer cancel2()
	bucket2, done := startLiveTest(ctx2, t)
	defer done()
	w2 := bucket2.Object("foo").NewWriter(ctx2)
	r2 := io.LimitReader(zReader{}, 3e8)
	h1 := sha1.New()
	tr := io.TeeReader(r2, h1)
	w2.Resume = true
	w2.ConcurrentUploads = 2
	if _, err := io.Copy(w2, tr); err != nil {
		t.Fatal(err)
	}
	if err := w2.Close(); err != nil {
		t.Fatal(err)
	}
	begSHA := fmt.Sprintf("%x", h1.Sum(nil))

	objR := bucket2.Object("foo").NewReader(ctx2)
	objR.ConcurrentDownloads = 3
	h2 := sha1.New()
	if _, err := io.Copy(h2, objR); err != nil {
		t.Fatal(err)
	}
	if err := objR.Close(); err != nil {
		t.Error(err)
	}
	endSHA := fmt.Sprintf("%x", h2.Sum(nil))
	if endSHA != begSHA {
		t.Errorf("got conflicting hashes: got %q, want %q", endSHA, begSHA)
	}
}

func TestAttrs(t *testing.T) {
	// TODO: test is flaky
	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	bucket, done := startLiveTest(ctx, t)
	defer done()

	attrlist := []*Attrs{
		&Attrs{
			ContentType: "jpeg/stream",
			Info: map[string]string{
				"one": "a",
				"two": "b",
			},
		},
		&Attrs{
			ContentType:  "application/MAGICFACE",
			LastModified: time.Unix(1464370149, 142000000),
			Info:         map[string]string{}, // can't be nil
		},
	}

	table := []struct {
		name string
		size int64
	}{
		{
			name: "small",
			size: 1e3,
		},
		{
			name: "large",
			size: 1e8 + 4,
		},
	}

	for _, e := range table {
		for _, attrs := range attrlist {
			w := bucket.Object(e.name).NewWriter(ctx).WithAttrs(attrs)
			if _, err := io.Copy(w, io.LimitReader(zReader{}, e.size)); err != nil {
				t.Error(err)
				continue
			}
			if err := w.Close(); err != nil {
				t.Error(err)
				continue
			}
			gotAttrs, err := bucket.Object(e.name).Attrs(ctx)
			if err != nil {
				t.Error(err)
				continue
			}
			if gotAttrs.ContentType != attrs.ContentType {
				t.Errorf("bad content-type for %s: got %q, want %q", e.name, gotAttrs.ContentType, attrs.ContentType)
			}
			if !reflect.DeepEqual(gotAttrs.Info, attrs.Info) {
				t.Errorf("bad info for %s: got %#v, want %#v", e.name, gotAttrs.Info, attrs.Info)
			}
			if !gotAttrs.LastModified.Equal(attrs.LastModified) {
				t.Errorf("bad lastmodified time for %s: got %v, want %v", e.name, gotAttrs.LastModified, attrs.LastModified)
			}
		}
	}
}

func TestAuthTokLive(t *testing.T) {
	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	bucket, done := startLiveTest(ctx, t)
	defer done()

	foo := "foo/bar"
	baz := "baz/bar"

	fw := bucket.Object(foo).NewWriter(ctx)
	io.Copy(fw, io.LimitReader(zReader{}, 1e5))
	if err := fw.Close(); err != nil {
		t.Fatal(err)
	}

	bw := bucket.Object(baz).NewWriter(ctx)
	io.Copy(bw, io.LimitReader(zReader{}, 1e5))
	if err := bw.Close(); err != nil {
		t.Fatal(err)
	}

	tok, err := bucket.AuthToken(ctx, "foo", time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	furl := fmt.Sprintf("%s?Authorization=%s", bucket.Object(foo).URL(), tok)
	frsp, err := http.Get(furl)
	if err != nil {
		t.Fatal(err)
	}
	if frsp.StatusCode != 200 {
		t.Fatalf("%s: got %s, want 200", furl, frsp.Status)
	}
	burl := fmt.Sprintf("%s?Authorization=%s", bucket.Object(baz).URL(), tok)
	brsp, err := http.Get(burl)
	if err != nil {
		t.Fatal(err)
	}
	if brsp.StatusCode != 401 {
		t.Fatalf("%s: got %s, want 401", burl, brsp.Status)
	}
}

func TestRangeReaderLive(t *testing.T) {
	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	bucket, done := startLiveTest(ctx, t)
	defer done()

	buf := &bytes.Buffer{}
	io.Copy(buf, io.LimitReader(zReader{}, 3e6))
	rs := bytes.NewReader(buf.Bytes())

	w := bucket.Object("foobar").NewWriter(ctx)
	if _, err := io.Copy(w, rs); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	table := []struct {
		offset, length int64
		size           int64 // expected actual size
	}{
		{
			offset: 1e6 - 50,
			length: 1e6 + 50,
			size:   1e6 + 50,
		},
		{
			offset: 0,
			length: -1,
			size:   3e6,
		},
		{
			offset: 2e6,
			length: -1,
			size:   1e6,
		},
		{
			offset: 2e6,
			length: 2e6,
			size:   1e6,
		},
	}

	for _, e := range table {
		if _, err := rs.Seek(e.offset, 0); err != nil {
			t.Error(err)
			continue
		}
		hw := sha1.New()
		var lr io.Reader
		lr = rs
		if e.length >= 0 {
			lr = io.LimitReader(rs, e.length)
		}
		if _, err := io.Copy(hw, lr); err != nil {
			t.Error(err)
			continue
		}
		r := bucket.Object("foobar").NewRangeReader(ctx, e.offset, e.length)
		defer r.Close()
		hr := sha1.New()
		read, err := io.Copy(hr, r)
		if err != nil {
			t.Error(err)
			continue
		}
		if read != e.size {
			t.Errorf("read %d bytes, wanted %d bytes", read, e.size)
		}
		got := fmt.Sprintf("%x", hr.Sum(nil))
		want := fmt.Sprintf("%x", hw.Sum(nil))
		if got != want {
			t.Errorf("bad hash, got %q, want %q", got, want)
		}
	}
}

func TestListObjectsWithPrefix(t *testing.T) {
	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	bucket, done := startLiveTest(ctx, t)
	defer done()

	foo := "foo/bar"
	baz := "baz/bar"

	fw := bucket.Object(foo).NewWriter(ctx)
	io.Copy(fw, io.LimitReader(zReader{}, 1e5))
	if err := fw.Close(); err != nil {
		t.Fatal(err)
	}

	bw := bucket.Object(baz).NewWriter(ctx)
	io.Copy(bw, io.LimitReader(zReader{}, 1e5))
	if err := bw.Close(); err != nil {
		t.Fatal(err)
	}

	// This is kind of a hack, but
	type lfun func(context.Context, int, *Cursor) ([]*Object, *Cursor, error)

	for _, f := range []lfun{bucket.ListObjects, bucket.ListCurrentObjects} {
		c := &Cursor{
			Name: "foo/",
		}
		var res []string
		for {
			objs, cur, err := f(ctx, 10, c)
			if err != nil && err != io.EOF {
				t.Fatalf("bucket.ListObjects: %v", err)
			}
			for _, o := range objs {
				attrs, err := o.Attrs(ctx)
				if err != nil {
					t.Errorf("(%v).Attrs: %v", o, err)
					continue
				}
				res = append(res, attrs.Name)
			}
			if err == io.EOF {
				break
			}
			c = cur
		}

		want := []string{"foo/bar"}
		if !reflect.DeepEqual(res, want) {
			t.Errorf("got %v, want %v", res, want)
		}
	}
}

type object struct {
	o   *Object
	err error
}

func countObjects(ctx context.Context, f func(context.Context, int, *Cursor) ([]*Object, *Cursor, error)) (int, error) {
	var got int
	ch := listObjects(ctx, f)
	for c := range ch {
		if c.err != nil {
			return 0, c.err
		}
		got++
	}
	return got, nil
}

func listObjects(ctx context.Context, f func(context.Context, int, *Cursor) ([]*Object, *Cursor, error)) <-chan object {
	ch := make(chan object)
	go func() {
		defer close(ch)
		var cur *Cursor
		for {
			objs, c, err := f(ctx, 100, cur)
			if err != nil && err != io.EOF {
				ch <- object{err: err}
				return
			}
			for _, o := range objs {
				ch <- object{o: o}
			}
			if err == io.EOF {
				return
			}
			cur = c
		}
	}()
	return ch
}

func startLiveTest(ctx context.Context, t *testing.T) (*Bucket, func()) {
	id := os.Getenv(apiID)
	key := os.Getenv(apiKey)
	if id == "" || key == "" {
		t.Skipf("B2_ACCOUNT_ID or B2_SECRET_KEY unset; skipping integration tests")
		return nil, nil
	}
	client, err := NewClient(ctx, id, key)
	if err != nil {
		t.Fatal(err)
		return nil, nil
	}
	bucket, err := client.NewBucket(ctx, id+bucketName, Private)
	if err != nil {
		t.Fatal(err)
		return nil, nil
	}
	f := func() {
		for c := range listObjects(ctx, bucket.ListObjects) {
			if c.err != nil {
				continue
			}
			if err := c.o.Delete(ctx); err != nil {
				t.Error(err)
			}
		}
		if err := bucket.Delete(ctx); err != nil {
			t.Error(err)
		}
	}
	return bucket, f
}
