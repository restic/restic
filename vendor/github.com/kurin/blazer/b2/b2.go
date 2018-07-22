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

// Package b2 provides a high-level interface to Backblaze's B2 cloud storage
// service.
//
// It is specifically designed to abstract away the Backblaze API details by
// providing familiar Go interfaces, specifically an io.Writer for object
// storage, and an io.Reader for object download.  Handling of transient
// errors, including network and authentication timeouts, is transparent.
//
// Methods that perform network requests accept a context.Context argument.
// Callers should use the context's cancellation abilities to end requests
// early, or to provide timeout or deadline guarantees.
//
// This package is in development and may make API changes.
package b2

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"sync"
	"time"
)

// Client is a Backblaze B2 client.
type Client struct {
	backend beRootInterface

	slock    sync.Mutex
	sWriters map[string]*Writer
	sReaders map[string]*Reader
	sMethods []methodCounter
	opts     clientOptions
}

// NewClient creates and returns a new Client with valid B2 service account
// tokens.
func NewClient(ctx context.Context, account, key string, opts ...ClientOption) (*Client, error) {
	c := &Client{
		backend: &beRoot{
			b2i: &b2Root{},
		},
		sMethods: []methodCounter{
			newMethodCounter(time.Minute, time.Second),
			newMethodCounter(time.Minute*5, time.Second),
			newMethodCounter(time.Hour, time.Minute),
			newMethodCounter(0, 0), // forever
		},
	}
	opts = append(opts, client(c))
	for _, f := range opts {
		f(&c.opts)
	}
	if err := c.backend.authorizeAccount(ctx, account, key, c.opts); err != nil {
		return nil, err
	}
	return c, nil
}

type clientOptions struct {
	client          *Client
	transport       http.RoundTripper
	failSomeUploads bool
	expireTokens    bool
	capExceeded     bool
	apiBase         string
	userAgents      []string
	writerOpts      []WriterOption
}

// A ClientOption allows callers to adjust various per-client settings.
type ClientOption func(*clientOptions)

// UserAgent sets the User-Agent HTTP header.  The default header is
// "blazer/<version>"; the value set here will be prepended to that.  This can
// be set multiple times.
//
// A user agent is generally of the form "<product>/<version> (<comments>)".
func UserAgent(agent string) ClientOption {
	return func(o *clientOptions) {
		o.userAgents = append(o.userAgents, agent)
	}
}

// APIBase returns a ClientOption specifying the URL root of API requests.
func APIBase(url string) ClientOption {
	return func(o *clientOptions) {
		o.apiBase = url
	}
}

// Transport sets the underlying HTTP transport mechanism.  If unset,
// http.DefaultTransport is used.
func Transport(rt http.RoundTripper) ClientOption {
	return func(c *clientOptions) {
		c.transport = rt
	}
}

// FailSomeUploads requests intermittent upload failures from the B2 service.
// This is mostly useful for testing.
func FailSomeUploads() ClientOption {
	return func(c *clientOptions) {
		c.failSomeUploads = true
	}
}

// ExpireSomeAuthTokens requests intermittent authentication failures from the
// B2 service.
func ExpireSomeAuthTokens() ClientOption {
	return func(c *clientOptions) {
		c.expireTokens = true
	}
}

// ForceCapExceeded requests a cap limit from the B2 service.  This causes all
// uploads to be treated as if they would exceed the configure B2 capacity.
func ForceCapExceeded() ClientOption {
	return func(c *clientOptions) {
		c.capExceeded = true
	}
}

func client(cl *Client) ClientOption {
	return func(c *clientOptions) {
		c.client = cl
	}
}

type clientTransport struct {
	client *Client
	rt     http.RoundTripper
}

func (ct *clientTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	m := r.Header.Get("X-Blazer-Method")
	t := ct.rt
	if t == nil {
		t = http.DefaultTransport
	}
	b := time.Now()
	resp, err := t.RoundTrip(r)
	e := time.Now()
	if err != nil {
		return resp, err
	}
	if m != "" && ct.client != nil {
		ct.client.slock.Lock()
		m := method{
			name:     m,
			duration: e.Sub(b),
			status:   resp.StatusCode,
		}
		for _, counter := range ct.client.sMethods {
			counter.record(m)
		}
		ct.client.slock.Unlock()
	}
	return resp, nil
}

// Bucket is a reference to a B2 bucket.
type Bucket struct {
	b beBucketInterface
	r beRootInterface

	c       *Client
	urlPool *urlPool
}

type BucketType string

const (
	UnknownType BucketType = ""
	Private                = "allPrivate"
	Public                 = "allPublic"
	Snapshot               = "snapshot"
)

// BucketAttrs holds a bucket's metadata attributes.
type BucketAttrs struct {
	// Type lists or sets the new bucket type.  If Type is UnknownType during a
	// bucket.Update, the type is not changed.
	Type BucketType

	// Info records user data, limited to ten keys.  If nil during a
	// bucket.Update, the existing bucket info is not modified.  A bucket's
	// metadata can be removed by updating with an empty map.
	Info map[string]string

	// Reports or sets bucket lifecycle rules.  If nil during a bucket.Update,
	// the rules are not modified.  A bucket's rules can be removed by updating
	// with an empty slice.
	LifecycleRules []LifecycleRule
}

// A LifecycleRule describes an object's life cycle, namely how many days after
// uploading an object should be hidden, and after how many days hidden an
// object should be deleted.  Multiple rules may not apply to the same file or
// set of files.  Be careful when using this feature; it can (is designed to)
// delete your data.
type LifecycleRule struct {
	// Prefix specifies all the files in the bucket to which this rule applies.
	Prefix string

	// DaysUploadedUntilHidden specifies the number of days after which a file
	// will automatically be hidden.  0 means "do not automatically hide new
	// files".
	DaysNewUntilHidden int

	// DaysHiddenUntilDeleted specifies the number of days after which a hidden
	// file is deleted.  0 means "do not automatically delete hidden files".
	DaysHiddenUntilDeleted int
}

type b2err struct {
	err              error
	notFoundErr      bool
	isUpdateConflict bool
}

func (e b2err) Error() string {
	return e.err.Error()
}

// IsNotExist reports whether a given error indicates that an object or bucket
// does not exist.
func IsNotExist(err error) bool {
	berr, ok := err.(b2err)
	if !ok {
		return false
	}
	return berr.notFoundErr
}

const uploadURLPoolSize = 100

type urlPool struct {
	ch chan beURLInterface
}

func newURLPool() *urlPool {
	return &urlPool{ch: make(chan beURLInterface, uploadURLPoolSize)}
}

func (p *urlPool) get() beURLInterface {
	select {
	case ue := <-p.ch:
		// if the channel has an upload URL available, use that
		return ue
	default:
		// otherwise return nil, a new upload URL needs to be generated
		return nil
	}
}

func (p *urlPool) put(u beURLInterface) {
	select {
	case p.ch <- u:
		// put the URL back if possible
	default:
		// if the channel is full, throw it away
	}
}

// Bucket returns a bucket if it exists.
func (c *Client) Bucket(ctx context.Context, name string) (*Bucket, error) {
	buckets, err := c.backend.listBuckets(ctx)
	if err != nil {
		return nil, err
	}
	for _, bucket := range buckets {
		if bucket.name() == name {
			return &Bucket{
				b:       bucket,
				r:       c.backend,
				c:       c,
				urlPool: newURLPool(),
			}, nil
		}
	}
	return nil, b2err{
		err:         fmt.Errorf("%s: bucket not found", name),
		notFoundErr: true,
	}
}

// NewBucket returns a bucket.  The bucket is created with the given attributes
// if it does not already exist.  If attrs is nil, it is created as a private
// bucket with no info metadata and no lifecycle rules.
func (c *Client) NewBucket(ctx context.Context, name string, attrs *BucketAttrs) (*Bucket, error) {
	buckets, err := c.backend.listBuckets(ctx)
	if err != nil {
		return nil, err
	}
	for _, bucket := range buckets {
		if bucket.name() == name {
			return &Bucket{
				b:       bucket,
				r:       c.backend,
				c:       c,
				urlPool: newURLPool(),
			}, nil
		}
	}
	if attrs == nil {
		attrs = &BucketAttrs{Type: Private}
	}
	b, err := c.backend.createBucket(ctx, name, string(attrs.Type), attrs.Info, attrs.LifecycleRules)
	if err != nil {
		return nil, err
	}
	return &Bucket{
		b:       b,
		r:       c.backend,
		c:       c,
		urlPool: newURLPool(),
	}, err
}

// ListBuckets returns all the available buckets.
func (c *Client) ListBuckets(ctx context.Context) ([]*Bucket, error) {
	bs, err := c.backend.listBuckets(ctx)
	if err != nil {
		return nil, err
	}
	var buckets []*Bucket
	for _, b := range bs {
		buckets = append(buckets, &Bucket{
			b:       b,
			r:       c.backend,
			c:       c,
			urlPool: newURLPool(),
		})
	}
	return buckets, nil
}

// IsUpdateConflict reports whether a given error is the result of a bucket
// update conflict.
func IsUpdateConflict(err error) bool {
	e, ok := err.(b2err)
	if !ok {
		return false
	}
	return e.isUpdateConflict
}

// Update modifies the given bucket with new attributes.  It is possible that
// this method could fail with an update conflict, in which case you should
// retrieve the latest bucket attributes with Attrs and try again.
func (b *Bucket) Update(ctx context.Context, attrs *BucketAttrs) error {
	return b.b.updateBucket(ctx, attrs)
}

// Attrs retrieves and returns the current bucket's attributes.
func (b *Bucket) Attrs(ctx context.Context) (*BucketAttrs, error) {
	bucket, err := b.c.Bucket(ctx, b.Name())
	if err != nil {
		return nil, err
	}
	b.b = bucket.b
	return b.b.attrs(), nil
}

var bNotExist = regexp.MustCompile("Bucket.*does not exist")

// Delete removes a bucket.  The bucket must be empty.
func (b *Bucket) Delete(ctx context.Context) error {
	err := b.b.deleteBucket(ctx)
	if err == nil {
		return err
	}
	// So, the B2 documentation disagrees with the implementation here, and the
	// error code is not really helpful.  If the bucket doesn't exist, the error is
	// 400, not 404, and the string is "Bucket <name> does not exist".  However, the
	// documentation says it will be "Bucket id <name> does not exist".  In case
	// they update the implementation to match the documentation, we're just going
	// to regexp over the error message and hope it's okay.
	if bNotExist.MatchString(err.Error()) {
		return b2err{
			err:         err,
			notFoundErr: true,
		}
	}
	return err
}

// BaseURL returns the base URL to use for all files uploaded to this bucket.
func (b *Bucket) BaseURL() string {
	return b.b.baseURL()
}

// Name returns the bucket's name.
func (b *Bucket) Name() string {
	return b.b.name()
}

// Object represents a B2 object.
type Object struct {
	attrs *Attrs
	name  string
	f     beFileInterface
	b     *Bucket
}

// Attrs holds an object's metadata.
type Attrs struct {
	Name            string            // Not used on upload.
	Size            int64             // Not used on upload.
	ContentType     string            // Used on upload, default is "application/octet-stream".
	Status          ObjectState       // Not used on upload.
	UploadTimestamp time.Time         // Not used on upload.
	SHA1            string            // Can be "none" for large files.  If set on upload, will be used for large files.
	LastModified    time.Time         // If present, and there are fewer than 10 keys in the Info field, this is saved on upload.
	Info            map[string]string // Save arbitrary metadata on upload, but limited to 10 keys.
}

// Name returns an object's name
func (o *Object) Name() string {
	return o.name
}

// Attrs returns an object's attributes.
func (o *Object) Attrs(ctx context.Context) (*Attrs, error) {
	if err := o.ensure(ctx); err != nil {
		return nil, err
	}
	fi, err := o.f.getFileInfo(ctx)
	if err != nil {
		return nil, err
	}
	name, sha, size, ct, info, st, stamp := fi.stats()
	var state ObjectState
	switch st {
	case "upload":
		state = Uploaded
	case "start":
		state = Started
	case "hide":
		state = Hider
	case "folder":
		state = Folder
	}
	var mtime time.Time
	if v, ok := info["src_last_modified_millis"]; ok {
		ms, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return nil, err
		}
		mtime = time.Unix(ms/1e3, (ms%1e3)*1e6)
		delete(info, "src_last_modified_millis")
	}
	if v, ok := info["large_file_sha1"]; ok {
		sha = v
	}
	return &Attrs{
		Name:            name,
		Size:            size,
		ContentType:     ct,
		UploadTimestamp: stamp,
		SHA1:            sha,
		Info:            info,
		Status:          state,
		LastModified:    mtime,
	}, nil
}

// ObjectState represents the various states an object can be in.
type ObjectState int

const (
	Unknown ObjectState = iota
	// Started represents a large upload that has been started but not finished
	// or canceled.
	Started
	// Uploaded represents an object that has finished uploading and is complete.
	Uploaded
	// Hider represents an object that exists only to hide another object.  It
	// cannot in itself be downloaded and, in particular, is not a hidden object.
	Hider

	// Folder is a special state given to non-objects that are returned during a
	// List call with a ListDelimiter option.
	Folder
)

// Object returns a reference to the named object in the bucket.  Hidden
// objects cannot be referenced in this manner; they can only be found by
// finding the appropriate reference in ListObjects.
func (b *Bucket) Object(name string) *Object {
	return &Object{
		name: name,
		b:    b,
	}
}

// URL returns the full URL to the given object.
func (o *Object) URL() string {
	return fmt.Sprintf("%s/file/%s/%s", o.b.BaseURL(), o.b.Name(), o.name)
}

// NewWriter returns a new writer for the given object.  Objects that are
// overwritten are not deleted, but are "hidden".
//
// Callers must close the writer when finished and check the error status.
func (o *Object) NewWriter(ctx context.Context, opts ...WriterOption) *Writer {
	ctx, cancel := context.WithCancel(ctx)
	w := &Writer{
		o:      o,
		name:   o.name,
		ctx:    ctx,
		cancel: cancel,
	}
	for _, f := range o.b.c.opts.writerOpts {
		f(w)
	}
	for _, f := range opts {
		f(w)
	}
	return w
}

// NewRangeReader returns a reader for the given object, reading up to length
// bytes.  If length is negative, the rest of the object is read.
func (o *Object) NewRangeReader(ctx context.Context, offset, length int64) *Reader {
	ctx, cancel := context.WithCancel(ctx)
	return &Reader{
		ctx:    ctx,
		cancel: cancel,
		o:      o,
		name:   o.name,
		chunks: make(map[int]*rchunk),
		length: length,
		offset: offset,
	}
}

// NewReader returns a reader for the given object.
func (o *Object) NewReader(ctx context.Context) *Reader {
	return o.NewRangeReader(ctx, 0, -1)
}

func (o *Object) ensure(ctx context.Context) error {
	if o.f == nil {
		f, err := o.b.getObject(ctx, o.name)
		if err != nil {
			return err
		}
		o.f = f.f
	}
	return nil
}

// Delete removes the given object.
func (o *Object) Delete(ctx context.Context) error {
	if err := o.ensure(ctx); err != nil {
		return err
	}
	return o.f.deleteFileVersion(ctx)
}

// Cursor is passed to ListObjects to return subsequent pages.
//
// DEPRECATED.  Will be removed in a future release.
type Cursor struct {
	// Prefix limits the listed objects to those that begin with this string.
	Prefix string

	// Delimiter denotes the path separator.  If set, object listings will be
	// truncated at this character.
	//
	// For example, if the bucket contains objects foo/bar, foo/baz, and foo,
	// then a delimiter of "/" will cause the listing to return "foo" and "foo/".
	// Otherwise, the listing would have returned all object names.
	//
	// Note that objects returned that end in the delimiter may not be actual
	// objects, e.g. you cannot read from (or write to, or delete) an object "foo/",
	// both because no actual object exists and because B2 disallows object names
	// that end with "/".  If you want to ensure that all objects returned by
	// ListObjects and ListCurrentObjects are actual objects, leave this unset.
	Delimiter string

	name string
	id   string
}

// ListObjects returns all objects in the bucket, including multiple versions
// of the same object.  Cursor may be nil; when passed to a subsequent query,
// it will continue the listing.
//
// ListObjects will return io.EOF when there are no objects left in the bucket,
// however it may do so concurrently with the last objects.
//
// DEPRECATED.  Will be removed in a future release.
func (b *Bucket) ListObjects(ctx context.Context, count int, c *Cursor) ([]*Object, *Cursor, error) {
	if c == nil {
		c = &Cursor{}
	}
	fs, name, id, err := b.b.listFileVersions(ctx, count, c.name, c.id, c.Prefix, c.Delimiter)
	if err != nil {
		return nil, nil, err
	}
	var next *Cursor
	if name != "" && id != "" {
		next = &Cursor{
			Prefix:    c.Prefix,
			Delimiter: c.Delimiter,
			name:      name,
			id:        id,
		}
	}
	var objects []*Object
	for _, f := range fs {
		objects = append(objects, &Object{
			name: f.name(),
			f:    f,
			b:    b,
		})
	}
	var rtnErr error
	if len(objects) == 0 || next == nil {
		rtnErr = io.EOF
	}
	return objects, next, rtnErr
}

// ListCurrentObjects is similar to ListObjects, except that it returns only
// current, unhidden objects in the bucket.
//
// DEPRECATED.  Will be removed in a future release.
func (b *Bucket) ListCurrentObjects(ctx context.Context, count int, c *Cursor) ([]*Object, *Cursor, error) {
	if c == nil {
		c = &Cursor{}
	}
	fs, name, err := b.b.listFileNames(ctx, count, c.name, c.Prefix, c.Delimiter)
	if err != nil {
		return nil, nil, err
	}
	var next *Cursor
	if name != "" {
		next = &Cursor{
			Prefix:    c.Prefix,
			Delimiter: c.Delimiter,
			name:      name,
		}
	}
	var objects []*Object
	for _, f := range fs {
		objects = append(objects, &Object{
			name: f.name(),
			f:    f,
			b:    b,
		})
	}
	var rtnErr error
	if len(objects) == 0 || next == nil {
		rtnErr = io.EOF
	}
	return objects, next, rtnErr
}

// ListUnfinishedLargeFiles lists any objects that correspond to large file uploads that haven't been completed.
// This can happen for example when an upload is interrupted.
//
// DEPRECATED.  Will be removed in a future release.
func (b *Bucket) ListUnfinishedLargeFiles(ctx context.Context, count int, c *Cursor) ([]*Object, *Cursor, error) {
	if c == nil {
		c = &Cursor{}
	}
	fs, name, err := b.b.listUnfinishedLargeFiles(ctx, count, c.name)
	if err != nil {
		return nil, nil, err
	}
	var next *Cursor
	if name != "" {
		next = &Cursor{
			name: name,
		}
	}
	var objects []*Object
	for _, f := range fs {
		objects = append(objects, &Object{
			name: f.name(),
			f:    f,
			b:    b,
		})
	}
	var rtnErr error
	if len(objects) == 0 || next == nil {
		rtnErr = io.EOF
	}
	return objects, next, rtnErr
}

// Hide hides the object from name-based listing.
func (o *Object) Hide(ctx context.Context) error {
	if err := o.ensure(ctx); err != nil {
		return err
	}
	_, err := o.b.b.hideFile(ctx, o.name)
	return err
}

// Reveal unhides (if hidden) the named object.  If there are multiple objects
// of a given name, it will reveal the most recent.
func (b *Bucket) Reveal(ctx context.Context, name string) error {
	cur := &Cursor{
		name: name,
	}
	objs, _, err := b.ListObjects(ctx, 1, cur)
	if err != nil && err != io.EOF {
		return err
	}
	if len(objs) < 1 || objs[0].name != name {
		return b2err{err: fmt.Errorf("%s: not found", name), notFoundErr: true}
	}
	obj := objs[0]
	if obj.f.status() != "hide" {
		return nil
	}
	return obj.Delete(ctx)
}

// I don't want to import all of ioutil for this.
type discard struct{}

func (discard) Write(p []byte) (int, error) {
	return len(p), nil
}

func (b *Bucket) getObject(ctx context.Context, name string) (*Object, error) {
	fr, err := b.b.downloadFileByName(ctx, name, 0, 1)
	if err != nil {
		return nil, err
	}
	io.Copy(discard{}, fr)
	fr.Close()
	return &Object{
		name: name,
		f:    b.b.file(fr.id(), name),
		b:    b,
	}, nil
}

// AuthToken returns an authorization token that can be used to access objects
// in a private bucket.  Only objects that begin with prefix can be accessed.
// The token expires after the given duration.
func (b *Bucket) AuthToken(ctx context.Context, prefix string, valid time.Duration) (string, error) {
	return b.b.getDownloadAuthorization(ctx, prefix, valid, "")
}

// AuthURL returns a URL for the given object with embedded token and,
// possibly, b2ContentDisposition arguments.  Leave b2cd blank for no content
// disposition.
func (o *Object) AuthURL(ctx context.Context, valid time.Duration, b2cd string) (*url.URL, error) {
	token, err := o.b.b.getDownloadAuthorization(ctx, o.name, valid, b2cd)
	if err != nil {
		return nil, err
	}
	urlString := fmt.Sprintf("%s?Authorization=%s", o.URL(), url.QueryEscape(token))
	if b2cd != "" {
		urlString = fmt.Sprintf("%s&b2ContentDisposition=%s", urlString, url.QueryEscape(b2cd))
	}
	u, err := url.Parse(urlString)
	if err != nil {
		return nil, err
	}
	return u, nil
}
