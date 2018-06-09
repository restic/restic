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
	"context"
	"crypto/rand"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kurin/blazer/x/transport"
)

const (
	apiID  = "B2_ACCOUNT_ID"
	apiKey = "B2_SECRET_KEY"

	errVar = "B2_TRANSIENT_ERRORS"
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

	lobj, wshaL, err := writeFile(ctx, bucket, largeFileName, 5e6+5e4, 5e6)
	if err != nil {
		t.Fatal(err)
	}

	if err := readFile(ctx, lobj, wshaL, 1e6, 10); err != nil {
		t.Error(err)
	}

	if err := readFile(ctx, sobj, wsha, 1e5, 10); err != nil {
		t.Error(err)
	}

	iter := bucket.List(ctx, ListHidden())
	for iter.Next() {
		if err := iter.Object().Delete(ctx); err != nil {
			t.Error(err)
		}
	}
	if err := iter.Err(); err != nil {
		t.Error(err)
	}
}

func TestReaderFromLive(t *testing.T) {
	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	bucket, done := startLiveTest(ctx, t)
	defer done()

	table := []struct {
		size, pos      int64
		csize, writers int
	}{
		{
			// that it works at all
			size: 10,
		},
		{
			// large uploads
			size:    15e6 + 10,
			csize:   5e6,
			writers: 2,
		},
		{
			// an excess of writers
			size:    50e6,
			csize:   5e6,
			writers: 12,
		},
		{
			// with offset, seeks back to start after turning it into a ReaderAt
			size: 250,
			pos:  50,
		},
	}

	for i, e := range table {
		rs := &zReadSeeker{pos: e.pos, size: e.size}
		o := bucket.Object(fmt.Sprintf("writer.%d", i))
		w := o.NewWriter(ctx)
		w.ChunkSize = e.csize
		w.ConcurrentUploads = e.writers
		n, err := w.ReadFrom(rs)
		if err != nil {
			t.Errorf("ReadFrom(): %v", err)
		}
		if n != e.size {
			t.Errorf("ReadFrom(): got %d bytes, wanted %d bytes", n, e.size)
		}
		if err := w.Close(); err != nil {
			t.Errorf("w.Close(): %v", err)
			continue
		}

		r := o.NewReader(ctx)
		h := sha1.New()
		rn, err := io.Copy(h, r)
		if err != nil {
			t.Errorf("Read from B2: %v", err)
		}
		if rn != n {
			t.Errorf("Read from B2: got %d bytes, want %d bytes", rn, n)
		}
		if err := r.Close(); err != nil {
			t.Errorf("r.Close(): %v", err)
		}

		hex := fmt.Sprintf("%x", h.Sum(nil))
		attrs, err := o.Attrs(ctx)
		if err != nil {
			t.Errorf("Attrs(): %v", err)
			continue
		}
		if attrs.SHA1 == "none" {
			continue
		}
		if hex != attrs.SHA1 {
			t.Errorf("SHA1: got %q, want %q", hex, attrs.SHA1)
		}
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

	got, err := countObjects(bucket.List(ctx))
	if err != nil {
		t.Error(err)
	}
	if got != 1 {
		t.Fatalf("got %d objects, wanted 1", got)
	}

	// When the hide marker and the object it's hiding were created within the
	// same second, they can be sorted in the wrong order, causing the object to
	// fail to be hidden.
	time.Sleep(1500 * time.Millisecond)

	// hide the file
	if err := obj.Hide(ctx); err != nil {
		t.Fatal(err)
	}

	got, err = countObjects(bucket.List(ctx))
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
	got, err = countObjects(bucket.List(ctx))
	if err != nil {
		t.Error(err)
	}
	if got != 1 {
		t.Fatalf("got %d objects, wanted 1", got)
	}
}

type cancelReader struct {
	r    io.Reader
	n, l int
	c    func()
}

func (c *cancelReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += n
	if c.n >= c.l {
		c.c()
	}
	return n, err
}

func TestResumeWriter(t *testing.T) {
	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	bucket, _ := startLiveTest(ctx, t)

	w := bucket.Object("foo").NewWriter(ctx)
	w.ChunkSize = 5e6
	r := &cancelReader{
		r: io.LimitReader(zReader{}, 15e6),
		l: 6e6,
		c: cancel,
	}
	if _, err := io.Copy(w, r); err != context.Canceled {
		t.Fatalf("io.Copy: wanted canceled context, got: %v", err)
	}

	ctx2 := context.Background()
	ctx2, cancel2 := context.WithTimeout(ctx2, 10*time.Minute)
	defer cancel2()
	bucket2, done := startLiveTest(ctx2, t)
	defer done()
	w2 := bucket2.Object("foo").NewWriter(ctx2)
	w2.ChunkSize = 5e6
	r2 := io.LimitReader(zReader{}, 15e6)
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
		&Attrs{
			ContentType: "arbitrarystring",
			Info: map[string]string{
				"spaces":  "string with spaces",
				"unicode": "日本語",
				"special": "&/!@_.~",
			},
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
			size: 5e6 + 4,
		},
	}

	for _, e := range table {
		for _, attrs := range attrlist {
			o := bucket.Object(e.name)
			w := o.NewWriter(ctx).WithAttrs(attrs)
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
			if err := o.Delete(ctx); err != nil {
				t.Errorf("Object(%q).Delete: %v", e.name, err)
			}
		}
	}
}

func TestFileBufferLive(t *testing.T) {
	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	bucket, done := startLiveTest(ctx, t)
	defer done()

	r := io.LimitReader(zReader{}, 1e6)
	w := bucket.Object("small").NewWriter(ctx)

	w.UseFileBuffer = true

	w.Write(nil)
	wb, ok := w.w.(*fileBuffer)
	if !ok {
		t.Fatalf("writer isn't using file buffer: %T", w.w)
	}
	smallTmpName := wb.f.Name()

	if _, err := io.Copy(w, r); err != nil {
		t.Errorf("creating small file: %v", err)
	}

	if err := w.Close(); err != nil {
		t.Errorf("w.Close(): %v", err)
	}

	if _, err := os.Stat(smallTmpName); !os.IsNotExist(err) {
		t.Errorf("tmp file exists (%s) or other error: %v", smallTmpName, err)
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
		{
			offset: 0,
			length: 4e6,
			size:   3e6,
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
			t.Errorf("NewRangeReader(_, %d, %d): read %d bytes, wanted %d bytes", e.offset, e.length, read, e.size)
		}
		got := fmt.Sprintf("%x", hr.Sum(nil))
		want := fmt.Sprintf("%x", hw.Sum(nil))
		if got != want {
			t.Errorf("NewRangeReader(_, %d, %d): got %q, want %q", e.offset, e.length, got, want)
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

	table := []struct {
		opts []ListOption
	}{
		{
			opts: []ListOption{
				ListPrefix("baz/"),
			},
		},
		{
			opts: []ListOption{
				ListPrefix("baz/"),
				ListHidden(),
			},
		},
	}

	for _, entry := range table {
		iter := bucket.List(ctx, entry.opts...)
		var res []string
		for iter.Next() {
			o := iter.Object()
			attrs, err := o.Attrs(ctx)
			if err != nil {
				t.Errorf("(%v).Attrs: %v", o, err)
				continue
			}
			res = append(res, attrs.Name)
		}
		if iter.Err() != nil {
			t.Errorf("iter.Err(): %v", iter.Err())
		}
		want := []string{"baz/bar"}
		if !reflect.DeepEqual(res, want) {
			t.Errorf("got %v, want %v", res, want)
		}
	}
}

func compare(a, b *BucketAttrs) bool {
	if a == nil {
		a = &BucketAttrs{}
	}
	if b == nil {
		b = &BucketAttrs{}
	}

	if a.Type != b.Type && !((a.Type == "" && b.Type == Private) || (a.Type == Private && b.Type == "")) {
		return false
	}

	if !reflect.DeepEqual(a.Info, b.Info) && (len(a.Info) > 0 || len(b.Info) > 0) {
		return false
	}

	return reflect.DeepEqual(a.LifecycleRules, b.LifecycleRules)
}

func TestNewBucket(t *testing.T) {
	id := os.Getenv(apiID)
	key := os.Getenv(apiKey)
	if id == "" || key == "" {
		t.Skipf("B2_ACCOUNT_ID or B2_SECRET_KEY unset; skipping integration tests")
	}
	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	client, err := NewClient(ctx, id, key)
	if err != nil {
		t.Fatal(err)
	}

	table := []struct {
		name  string
		attrs *BucketAttrs
	}{
		{
			name: "no-attrs",
		},
		{
			name: "only-rules",
			attrs: &BucketAttrs{
				LifecycleRules: []LifecycleRule{
					{
						Prefix:                 "whee/",
						DaysHiddenUntilDeleted: 30,
					},
					{
						Prefix:             "whoa/",
						DaysNewUntilHidden: 1,
					},
				},
			},
		},
		{
			name: "only-info",
			attrs: &BucketAttrs{
				Info: map[string]string{
					"this":  "that",
					"other": "thing",
				},
			},
		},
	}

	for _, ent := range table {
		bucket, err := client.NewBucket(ctx, id+"-"+ent.name, ent.attrs)
		if err != nil {
			t.Errorf("%s: NewBucket(%v): %v", ent.name, ent.attrs, err)
			continue
		}
		defer bucket.Delete(ctx)
		if err := bucket.Update(ctx, nil); err != nil {
			t.Errorf("%s: Update(ctx, nil): %v", ent.name, err)
			continue
		}
		attrs, err := bucket.Attrs(ctx)
		if err != nil {
			t.Errorf("%s: Attrs(ctx): %v", ent.name, err)
			continue
		}
		if !compare(attrs, ent.attrs) {
			t.Errorf("%s: attrs disagree: got %v, want %v", ent.name, attrs, ent.attrs)
		}
	}
}

func TestDuelingBuckets(t *testing.T) {
	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	bucket, done := startLiveTest(ctx, t)
	defer done()
	bucket2, done2 := startLiveTest(ctx, t)
	defer done2()

	attrs, err := bucket.Attrs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	attrs2, err := bucket2.Attrs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	attrs.Info["food"] = "yum"
	if err := bucket.Update(ctx, attrs); err != nil {
		t.Fatal(err)
	}

	attrs2.Info["nails"] = "not"
	if err := bucket2.Update(ctx, attrs2); !IsUpdateConflict(err) {
		t.Fatalf("bucket.Update should have failed with IsUpdateConflict; instead failed with %v", err)
	}

	attrs2, err = bucket2.Attrs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	attrs2.Info["nails"] = "not"
	if err := bucket2.Update(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if err := bucket2.Update(ctx, attrs2); err != nil {
		t.Fatal(err)
	}
}

func TestNotExist(t *testing.T) {
	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	bucket, done := startLiveTest(ctx, t)
	defer done()

	if _, err := bucket.Object("not there").Attrs(ctx); !IsNotExist(err) {
		t.Errorf("IsNotExist() on nonexistent object returned false (%v)", err)
	}
}

func TestWriteEmpty(t *testing.T) {
	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	bucket, done := startLiveTest(ctx, t)
	defer done()

	_, _, err := writeFile(ctx, bucket, smallFileName, 0, 1e8)
	if err != nil {
		t.Fatal(err)
	}
}

func TestAttrsNoRoundtrip(t *testing.T) {
	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	bucket, done := startLiveTest(ctx, t)
	defer done()

	_, _, err := writeFile(ctx, bucket, smallFileName, 1e6+42, 1e8)
	if err != nil {
		t.Fatal(err)
	}

	iter := bucket.List(ctx)
	iter.Next()
	obj := iter.Object()

	var trips int
	for range bucket.c.Status().table()["1m"] {
		trips++
	}
	attrs, err := obj.Attrs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if attrs.Name != smallFileName {
		t.Errorf("got the wrong object: got %q, want %q", attrs.Name, smallFileName)
	}

	var newTrips int
	for range bucket.c.Status().table()["1m"] {
		newTrips++
	}
	if trips != newTrips {
		t.Errorf("Attrs() should not have caused any net traffic, but it did: old %d, new %d", trips, newTrips)
	}
}

/*func TestAttrsFewRoundtrips(t *testing.T) {
	rt := &rtCounter{rt: defaultTransport}
	defaultTransport = rt
	defer func() {
		defaultTransport = rt.rt
	}()

	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	bucket, done := startLiveTest(ctx, t)
	defer done()

	_, _, err := writeFile(ctx, bucket, smallFileName, 42, 1e8)
	if err != nil {
		t.Fatal(err)
	}

	o := bucket.Object(smallFileName)
	trips := rt.trips
	attrs, err := o.Attrs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if attrs.Name != smallFileName {
		t.Errorf("got the wrong object: got %q, want %q", attrs.Name, smallFileName)
	}

	if trips != rt.trips {
		t.Errorf("Attrs(): too many round trips, got %d, want 1", rt.trips-trips)
	}
}*/

func TestSmallUploadsFewRoundtrips(t *testing.T) {
	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	bucket, done := startLiveTest(ctx, t)
	defer done()

	for i := 0; i < 10; i++ {
		_, _, err := writeFile(ctx, bucket, fmt.Sprintf("%s.%d", smallFileName, i), 42, 1e8)
		if err != nil {
			t.Fatal(err)
		}
	}
	si := bucket.c.Status()
	getURL := si.RPCs[0].CountByMethod()["b2_get_upload_url"]
	uploadFile := si.RPCs[0].CountByMethod()["b2_upload_file"]
	if getURL >= uploadFile {
		t.Errorf("too many calls to b2_get_upload_url")
	}
}

func TestDeleteWithoutName(t *testing.T) {
	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	bucket, done := startLiveTest(ctx, t)
	defer done()

	_, _, err := writeFile(ctx, bucket, smallFileName, 1e6+42, 1e8)
	if err != nil {
		t.Fatal(err)
	}

	if err := bucket.Object(smallFileName).Delete(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestListUnfinishedLargeFiles(t *testing.T) {
	ctx := context.Background()
	bucket, done := startLiveTest(ctx, t)
	defer done()

	w := bucket.Object(largeFileName).NewWriter(ctx)
	w.ChunkSize = 1e5
	if _, err := io.Copy(w, io.LimitReader(zReader{}, 1e6)); err != nil {
		t.Fatal(err)
	}
	iter := bucket.List(ctx, ListUnfinished())
	if !iter.Next() {
		t.Errorf("ListUnfinishedLargeFiles: got none, want 1 (error %v)", iter.Err())
	}
}

func TestReauthPreservesOptions(t *testing.T) {
	ctx := context.Background()
	bucket, done := startLiveTest(ctx, t)
	defer done()

	var first []ClientOption
	opts := bucket.r.(*beRoot).options
	for _, o := range opts {
		first = append(first, o)
	}

	if err := bucket.r.reauthorizeAccount(ctx); err != nil {
		t.Fatalf("reauthorizeAccount: %v", err)
	}

	second := bucket.r.(*beRoot).options
	if len(second) != len(first) {
		t.Fatalf("options mismatch: got %d options, wanted %d", len(second), len(first))
	}

	var f, s clientOptions
	for i := range first {
		first[i](&f)
		second[i](&s)
	}

	if !f.eq(s) {
		t.Errorf("options mismatch: got %v, want %v", s, f)
	}
}

type object struct {
	o   *Object
	err error
}

func countObjects(iter *ObjectIterator) (int, error) {
	var got int
	for iter.Next() {
		got++
	}
	return got, iter.Err()
}

var defaultTransport = http.DefaultTransport

type eofTripper struct {
	rt http.RoundTripper
	t  *testing.T
}

func (et eofTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := et.rt.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	resp.Body = &eofReadCloser{rc: resp.Body, t: et.t}
	return resp, nil
}

type eofReadCloser struct {
	rc  io.ReadCloser
	eof bool
	t   *testing.T
}

func (eof *eofReadCloser) Read(p []byte) (int, error) {
	n, err := eof.rc.Read(p)
	if err == io.EOF {
		eof.eof = true
	}
	return n, err
}

func (eof *eofReadCloser) Close() error {
	if !eof.eof {
		eof.t.Error("http body closed with bytes unread")
	}
	return eof.rc.Close()
}

// Checks that close is called.
type ccTripper struct {
	t     *testing.T
	rt    http.RoundTripper
	trips int64
}

func (cc *ccTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := cc.rt.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	atomic.AddInt64(&cc.trips, 1)
	resp.Body = &ccRC{ReadCloser: resp.Body, c: &cc.trips}
	return resp, err
}

func (cc *ccTripper) done() {
	if cc.trips != 0 {
		cc.t.Errorf("failed to close %d HTTP bodies", cc.trips)
	}
}

type ccRC struct {
	io.ReadCloser
	c *int64
}

func (cc *ccRC) Close() error {
	atomic.AddInt64(cc.c, -1)
	return cc.ReadCloser.Close()
}

var uniq string

func init() {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	uniq = hex.EncodeToString(b)
}

func startLiveTest(ctx context.Context, t *testing.T) (*Bucket, func()) {
	id := os.Getenv(apiID)
	key := os.Getenv(apiKey)
	if id == "" || key == "" {
		t.Skipf("B2_ACCOUNT_ID or B2_SECRET_KEY unset; skipping integration tests")
		return nil, nil
	}
	ccport := &ccTripper{rt: defaultTransport, t: t}
	tport := eofTripper{rt: ccport, t: t}
	errport := transport.WithFailures(tport, transport.FailureRate(.25), transport.MatchPathSubstring("/b2_get_upload_url"), transport.Response(503))
	client, err := NewClient(ctx, id, key, FailSomeUploads(), ExpireSomeAuthTokens(), Transport(errport), UserAgent("b2-test"), UserAgent("integration-test"))
	if err != nil {
		t.Fatal(err)
		return nil, nil
	}
	bucket, err := client.NewBucket(ctx, fmt.Sprintf("%s-%s-%s", id, bucketName, uniq), nil)
	if err != nil {
		t.Fatal(err)
		return nil, nil
	}
	f := func() {
		defer ccport.done()
		iter := bucket.List(ctx, ListHidden())
		for iter.Next() {
			if err := iter.Object().Delete(ctx); err != nil {
				t.Error(err)
			}
		}
		if err := iter.Err(); err != nil && !IsNotExist(err) {
			t.Errorf("%#v", err)
		}
		if err := bucket.Delete(ctx); err != nil && !IsNotExist(err) {
			t.Error(err)
		}
	}
	return bucket, f
}
