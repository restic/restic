//
// goamz - Go packages to interact with the Amazon Web Services.
//
//   https://wiki.ubuntu.com/goamz
//
// Copyright (c) 2011 Canonical Ltd.
//
// Written by Gustavo Niemeyer <gustavo.niemeyer@canonical.com>
//

package s3

import (
	"bytes"
	"crypto/md5"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"github.com/mitchellh/goamz/aws"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const debug = false

// The S3 type encapsulates operations with an S3 region.
type S3 struct {
	aws.Auth
	aws.Region
	HTTPClient func() *http.Client

	private byte // Reserve the right of using private data.
}

// The Bucket type encapsulates operations with an S3 bucket.
type Bucket struct {
	*S3
	Name string
}

// The Owner type represents the owner of the object in an S3 bucket.
type Owner struct {
	ID          string
	DisplayName string
}

var attempts = aws.AttemptStrategy{
	Min:   5,
	Total: 5 * time.Second,
	Delay: 200 * time.Millisecond,
}

// New creates a new S3.
func New(auth aws.Auth, region aws.Region) *S3 {
	return &S3{
		Auth:   auth,
		Region: region,
		HTTPClient: func() *http.Client {
			return http.DefaultClient
		},
		private: 0}
}

// Bucket returns a Bucket with the given name.
func (s3 *S3) Bucket(name string) *Bucket {
	if s3.Region.S3BucketEndpoint != "" || s3.Region.S3LowercaseBucket {
		name = strings.ToLower(name)
	}
	return &Bucket{s3, name}
}

var createBucketConfiguration = `<CreateBucketConfiguration xmlns="http://s3.amazonaws.com/doc/2006-03-01/">
  <LocationConstraint>%s</LocationConstraint>
</CreateBucketConfiguration>`

// locationConstraint returns an io.Reader specifying a LocationConstraint if
// required for the region.
//
// See http://goo.gl/bh9Kq for details.
func (s3 *S3) locationConstraint() io.Reader {
	constraint := ""
	if s3.Region.S3LocationConstraint {
		constraint = fmt.Sprintf(createBucketConfiguration, s3.Region.Name)
	}
	return strings.NewReader(constraint)
}

type ACL string

const (
	Private           = ACL("private")
	PublicRead        = ACL("public-read")
	PublicReadWrite   = ACL("public-read-write")
	AuthenticatedRead = ACL("authenticated-read")
	BucketOwnerRead   = ACL("bucket-owner-read")
	BucketOwnerFull   = ACL("bucket-owner-full-control")
)

// The ListBucketsResp type holds the results of a List buckets operation.
type ListBucketsResp struct {
	Buckets []Bucket `xml:">Bucket"`
}

// ListBuckets lists all buckets
//
// See: http://goo.gl/NqlyMN
func (s3 *S3) ListBuckets() (result *ListBucketsResp, err error) {
	req := &request{
		path: "/",
	}
	result = &ListBucketsResp{}
	for attempt := attempts.Start(); attempt.Next(); {
		err = s3.query(req, result)
		if !shouldRetry(err) {
			break
		}
	}
	if err != nil {
		return nil, err
	}
	// set S3 instance on buckets
	for i := range result.Buckets {
		result.Buckets[i].S3 = s3
	}
	return result, nil
}

// PutBucket creates a new bucket.
//
// See http://goo.gl/ndjnR for details.
func (b *Bucket) PutBucket(perm ACL) error {
	headers := map[string][]string{
		"x-amz-acl": {string(perm)},
	}
	req := &request{
		method:  "PUT",
		bucket:  b.Name,
		path:    "/",
		headers: headers,
		payload: b.locationConstraint(),
	}
	return b.S3.query(req, nil)
}

// DelBucket removes an existing S3 bucket. All objects in the bucket must
// be removed before the bucket itself can be removed.
//
// See http://goo.gl/GoBrY for details.
func (b *Bucket) DelBucket() (err error) {
	req := &request{
		method: "DELETE",
		bucket: b.Name,
		path:   "/",
	}
	for attempt := attempts.Start(); attempt.Next(); {
		err = b.S3.query(req, nil)
		if !shouldRetry(err) {
			break
		}
	}
	return err
}

// Get retrieves an object from an S3 bucket.
//
// See http://goo.gl/isCO7 for details.
func (b *Bucket) Get(path string) (data []byte, err error) {
	body, err := b.GetReader(path)
	if err != nil {
		return nil, err
	}
	data, err = ioutil.ReadAll(body)
	body.Close()
	return data, err
}

// GetReader retrieves an object from an S3 bucket.
// It is the caller's responsibility to call Close on rc when
// finished reading.
func (b *Bucket) GetReader(path string) (rc io.ReadCloser, err error) {
	resp, err := b.GetResponse(path)
	if resp != nil {
		return resp.Body, err
	}
	return nil, err
}

// GetResponse retrieves an object from an S3 bucket returning the http response
// It is the caller's responsibility to call Close on rc when
// finished reading.
func (b *Bucket) GetResponse(path string) (*http.Response, error) {
	return b.getResponseParams(path, nil)
}

// GetTorrent retrieves an Torrent object from an S3 bucket an io.ReadCloser.
// It is the caller's responsibility to call Close on rc when finished reading.
func (b *Bucket) GetTorrentReader(path string) (io.ReadCloser, error) {
	resp, err := b.getResponseParams(path, url.Values{"torrent": {""}})
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}

// GetTorrent retrieves an Torrent object from an S3, returning
// the torrent as a []byte.
func (b *Bucket) GetTorrent(path string) ([]byte, error) {
	body, err := b.GetTorrentReader(path)
	if err != nil {
		return nil, err
	}
	defer body.Close()

	return ioutil.ReadAll(body)
}

func (b *Bucket) getResponseParams(path string, params url.Values) (*http.Response, error) {
	req := &request{
		bucket: b.Name,
		path:   path,
		params: params,
	}
	err := b.S3.prepare(req)
	if err != nil {
		return nil, err
	}
	for attempt := attempts.Start(); attempt.Next(); {
		resp, err := b.S3.run(req, nil)
		if shouldRetry(err) && attempt.HasNext() {
			continue
		}
		if err != nil {
			return nil, err
		}
		return resp, nil
	}
	panic("unreachable")
}

func (b *Bucket) Head(path string) (*http.Response, error) {
	req := &request{
		method: "HEAD",
		bucket: b.Name,
		path:   path,
	}
	err := b.S3.prepare(req)
	if err != nil {
		return nil, err
	}
	for attempt := attempts.Start(); attempt.Next(); {
		resp, err := b.S3.run(req, nil)
		if shouldRetry(err) && attempt.HasNext() {
			continue
		}
		if err != nil {
			return nil, err
		}
		return resp, nil
	}
	panic("unreachable")
}

// Put inserts an object into the S3 bucket.
//
// See http://goo.gl/FEBPD for details.
func (b *Bucket) Put(path string, data []byte, contType string, perm ACL) error {
	body := bytes.NewBuffer(data)
	return b.PutReader(path, body, int64(len(data)), contType, perm)
}

/*
PutHeader - like Put, inserts an object into the S3 bucket.
Instead of Content-Type string, pass in custom headers to override defaults.
*/
func (b *Bucket) PutHeader(path string, data []byte, customHeaders map[string][]string, perm ACL) error {
	body := bytes.NewBuffer(data)
	return b.PutReaderHeader(path, body, int64(len(data)), customHeaders, perm)
}

// PutReader inserts an object into the S3 bucket by consuming data
// from r until EOF.
func (b *Bucket) PutReader(path string, r io.Reader, length int64, contType string, perm ACL) error {
	headers := map[string][]string{
		"Content-Length": {strconv.FormatInt(length, 10)},
		"Content-Type":   {contType},
		"x-amz-acl":      {string(perm)},
	}
	req := &request{
		method:  "PUT",
		bucket:  b.Name,
		path:    path,
		headers: headers,
		payload: r,
	}
	return b.S3.query(req, nil)
}

/*
PutReaderHeader - like PutReader, inserts an object into S3 from a reader.
Instead of Content-Type string, pass in custom headers to override defaults.
*/
func (b *Bucket) PutReaderHeader(path string, r io.Reader, length int64, customHeaders map[string][]string, perm ACL) error {
	// Default headers
	headers := map[string][]string{
		"Content-Length": {strconv.FormatInt(length, 10)},
		"Content-Type":   {"application/text"},
		"x-amz-acl":      {string(perm)},
	}

	// Override with custom headers
	for key, value := range customHeaders {
		headers[key] = value
	}

	req := &request{
		method:  "PUT",
		bucket:  b.Name,
		path:    path,
		headers: headers,
		payload: r,
	}
	return b.S3.query(req, nil)
}

/*
Copy - copy objects inside bucket
*/
func (b *Bucket) Copy(oldPath, newPath string, perm ACL) error {
	if !strings.HasPrefix(oldPath, "/") {
		oldPath = "/" + oldPath
	}

	req := &request{
		method: "PUT",
		bucket: b.Name,
		path:   newPath,
		headers: map[string][]string{
			"x-amz-copy-source": {amazonEscape("/" + b.Name + oldPath)},
			"x-amz-acl":         {string(perm)},
		},
	}

	err := b.S3.prepare(req)
	if err != nil {
		return err
	}

	for attempt := attempts.Start(); attempt.Next(); {
		_, err = b.S3.run(req, nil)
		if shouldRetry(err) && attempt.HasNext() {
			continue
		}
		if err != nil {
			return err
		}
		return nil
	}
	panic("unreachable")
}

// Del removes an object from the S3 bucket.
//
// See http://goo.gl/APeTt for details.
func (b *Bucket) Del(path string) error {
	req := &request{
		method: "DELETE",
		bucket: b.Name,
		path:   path,
	}
	return b.S3.query(req, nil)
}

type Object struct {
	Key string
}

type MultiObjectDeleteBody struct {
	XMLName xml.Name `xml:"Delete"`
	Quiet   bool
	Object  []Object
}

func base64md5(data []byte) string {
	h := md5.New()
	h.Write(data)
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

// MultiDel removes multiple objects from the S3 bucket efficiently.
// A maximum of 1000 keys at once may be specified.
//
// See http://goo.gl/WvA5sj for details.
func (b *Bucket) MultiDel(paths []string) error {
	// create XML payload
	v := MultiObjectDeleteBody{}
	v.Object = make([]Object, len(paths))
	for i, path := range paths {
		v.Object[i] = Object{path}
	}
	data, _ := xml.Marshal(v)

	// Content-MD5 is required
	md5hash := base64md5(data)
	req := &request{
		method:  "POST",
		bucket:  b.Name,
		path:    "/",
		params:  url.Values{"delete": {""}},
		headers: http.Header{"Content-MD5": {md5hash}},
		payload: bytes.NewReader(data),
	}

	return b.S3.query(req, nil)
}

// The ListResp type holds the results of a List bucket operation.
type ListResp struct {
	Name       string
	Prefix     string
	Delimiter  string
	Marker     string
	NextMarker string
	MaxKeys    int
	// IsTruncated is true if the results have been truncated because
	// there are more keys and prefixes than can fit in MaxKeys.
	// N.B. this is the opposite sense to that documented (incorrectly) in
	// http://goo.gl/YjQTc
	IsTruncated    bool
	Contents       []Key
	CommonPrefixes []string `xml:">Prefix"`
}

// The Key type represents an item stored in an S3 bucket.
type Key struct {
	Key          string
	LastModified string
	Size         int64
	// ETag gives the hex-encoded MD5 sum of the contents,
	// surrounded with double-quotes.
	ETag         string
	StorageClass string
	Owner        Owner
}

// List returns information about objects in an S3 bucket.
//
// The prefix parameter limits the response to keys that begin with the
// specified prefix.
//
// The delim parameter causes the response to group all of the keys that
// share a common prefix up to the next delimiter in a single entry within
// the CommonPrefixes field. You can use delimiters to separate a bucket
// into different groupings of keys, similar to how folders would work.
//
// The marker parameter specifies the key to start with when listing objects
// in a bucket. Amazon S3 lists objects in alphabetical order and
// will return keys alphabetically greater than the marker.
//
// The max parameter specifies how many keys + common prefixes to return in
// the response. The default is 1000.
//
// For example, given these keys in a bucket:
//
//     index.html
//     index2.html
//     photos/2006/January/sample.jpg
//     photos/2006/February/sample2.jpg
//     photos/2006/February/sample3.jpg
//     photos/2006/February/sample4.jpg
//
// Listing this bucket with delimiter set to "/" would yield the
// following result:
//
//     &ListResp{
//         Name:      "sample-bucket",
//         MaxKeys:   1000,
//         Delimiter: "/",
//         Contents:  []Key{
//             {Key: "index.html", "index2.html"},
//         },
//         CommonPrefixes: []string{
//             "photos/",
//         },
//     }
//
// Listing the same bucket with delimiter set to "/" and prefix set to
// "photos/2006/" would yield the following result:
//
//     &ListResp{
//         Name:      "sample-bucket",
//         MaxKeys:   1000,
//         Delimiter: "/",
//         Prefix:    "photos/2006/",
//         CommonPrefixes: []string{
//             "photos/2006/February/",
//             "photos/2006/January/",
//         },
//     }
//
// See http://goo.gl/YjQTc for details.
func (b *Bucket) List(prefix, delim, marker string, max int) (result *ListResp, err error) {
	params := map[string][]string{
		"prefix":    {prefix},
		"delimiter": {delim},
		"marker":    {marker},
	}
	if max != 0 {
		params["max-keys"] = []string{strconv.FormatInt(int64(max), 10)}
	}
	req := &request{
		bucket: b.Name,
		params: params,
	}
	result = &ListResp{}
	for attempt := attempts.Start(); attempt.Next(); {
		err = b.S3.query(req, result)
		if !shouldRetry(err) {
			break
		}
	}
	if err != nil {
		return nil, err
	}
	return result, nil
}

// Returns a mapping of all key names in this bucket to Key objects
func (b *Bucket) GetBucketContents() (*map[string]Key, error) {
	bucket_contents := map[string]Key{}
	prefix := ""
	path_separator := ""
	marker := ""
	for {
		contents, err := b.List(prefix, path_separator, marker, 1000)
		if err != nil {
			return &bucket_contents, err
		}
		last_key := ""
		for _, key := range contents.Contents {
			bucket_contents[key.Key] = key
			last_key = key.Key
		}
		if contents.IsTruncated {
			marker = contents.NextMarker
			if marker == "" {
				// From the s3 docs: If response does not include the
				// NextMarker and it is truncated, you can use the value of the
				// last Key in the response as the marker in the subsequent
				// request to get the next set of object keys.
				marker = last_key
			}
		} else {
			break
		}
	}

	return &bucket_contents, nil
}

// Get metadata from the key without returning the key content
func (b *Bucket) GetKey(path string) (*Key, error) {
	req := &request{
		bucket: b.Name,
		path:   path,
		method: "HEAD",
	}
	err := b.S3.prepare(req)
	if err != nil {
		return nil, err
	}
	key := &Key{}
	for attempt := attempts.Start(); attempt.Next(); {
		resp, err := b.S3.run(req, nil)
		if shouldRetry(err) && attempt.HasNext() {
			continue
		}
		if err != nil {
			return nil, err
		}
		key.Key = path
		key.LastModified = resp.Header.Get("Last-Modified")
		key.ETag = resp.Header.Get("ETag")
		contentLength := resp.Header.Get("Content-Length")
		size, err := strconv.ParseInt(contentLength, 10, 64)
		if err != nil {
			return key, fmt.Errorf("bad s3 content-length %v: %v",
				contentLength, err)
		}
		key.Size = size
		return key, nil
	}
	panic("unreachable")
}

// URL returns a non-signed URL that allows retriving the
// object at path. It only works if the object is publicly
// readable (see SignedURL).
func (b *Bucket) URL(path string) string {
	req := &request{
		bucket: b.Name,
		path:   path,
	}
	err := b.S3.prepare(req)
	if err != nil {
		panic(err)
	}
	u, err := req.url(true)
	if err != nil {
		panic(err)
	}
	u.RawQuery = ""
	return u.String()
}

// SignedURL returns a signed URL that allows anyone holding the URL
// to retrieve the object at path. The signature is valid until expires.
func (b *Bucket) SignedURL(path string, expires time.Time) string {
	req := &request{
		bucket: b.Name,
		path:   path,
		params: url.Values{"Expires": {strconv.FormatInt(expires.Unix(), 10)}},
	}
	err := b.S3.prepare(req)
	if err != nil {
		panic(err)
	}
	u, err := req.url(true)
	if err != nil {
		panic(err)
	}
	return u.String()
}

type request struct {
	method   string
	bucket   string
	path     string
	signpath string
	params   url.Values
	headers  http.Header
	baseurl  string
	payload  io.Reader
	prepared bool
}

// amazonShouldEscape returns true if byte should be escaped
func amazonShouldEscape(c byte) bool {
	return !((c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') ||
		(c >= '0' && c <= '9') || c == '_' || c == '-' || c == '~' || c == '.' || c == '/' || c == ':')
}

// amazonEscape does uri escaping exactly as Amazon does
func amazonEscape(s string) string {
	hexCount := 0

	for i := 0; i < len(s); i++ {
		if amazonShouldEscape(s[i]) {
			hexCount++
		}
	}

	if hexCount == 0 {
		return s
	}

	t := make([]byte, len(s)+2*hexCount)
	j := 0
	for i := 0; i < len(s); i++ {
		if c := s[i]; amazonShouldEscape(c) {
			t[j] = '%'
			t[j+1] = "0123456789ABCDEF"[c>>4]
			t[j+2] = "0123456789ABCDEF"[c&15]
			j += 3
		} else {
			t[j] = s[i]
			j++
		}
	}
	return string(t)
}

// url returns url to resource, either full (with host/scheme) or
// partial for HTTP request
func (req *request) url(full bool) (*url.URL, error) {
	u, err := url.Parse(req.baseurl)
	if err != nil {
		return nil, fmt.Errorf("bad S3 endpoint URL %q: %v", req.baseurl, err)
	}

	u.Opaque = amazonEscape(req.path)
	if full {
		u.Opaque = "//" + u.Host + u.Opaque
	}
	u.RawQuery = req.params.Encode()

	return u, nil
}

// query prepares and runs the req request.
// If resp is not nil, the XML data contained in the response
// body will be unmarshalled on it.
func (s3 *S3) query(req *request, resp interface{}) error {
	err := s3.prepare(req)
	if err == nil {
		var httpResponse *http.Response
		httpResponse, err = s3.run(req, resp)
		if resp == nil && httpResponse != nil {
			httpResponse.Body.Close()
		}
	}
	return err
}

// prepare sets up req to be delivered to S3.
func (s3 *S3) prepare(req *request) error {
	if !req.prepared {
		req.prepared = true
		if req.method == "" {
			req.method = "GET"
		}
		// Copy so they can be mutated without affecting on retries.
		params := make(url.Values)
		headers := make(http.Header)
		for k, v := range req.params {
			params[k] = v
		}
		for k, v := range req.headers {
			headers[k] = v
		}
		req.params = params
		req.headers = headers
		if !strings.HasPrefix(req.path, "/") {
			req.path = "/" + req.path
		}
		req.signpath = req.path

		if req.bucket != "" {
			req.baseurl = s3.Region.S3BucketEndpoint
			if req.baseurl == "" {
				// Use the path method to address the bucket.
				req.baseurl = s3.Region.S3Endpoint
				req.path = "/" + req.bucket + req.path
			} else {
				// Just in case, prevent injection.
				if strings.IndexAny(req.bucket, "/:@") >= 0 {
					return fmt.Errorf("bad S3 bucket: %q", req.bucket)
				}
				req.baseurl = strings.Replace(req.baseurl, "${bucket}", req.bucket, -1)
			}
			req.signpath = "/" + req.bucket + req.signpath
		} else {
			req.baseurl = s3.Region.S3Endpoint
		}
	}

	// Always sign again as it's not clear how far the
	// server has handled a previous attempt.
	u, err := url.Parse(req.baseurl)
	if err != nil {
		return fmt.Errorf("bad S3 endpoint URL %q: %v", req.baseurl, err)
	}
	req.headers["Host"] = []string{u.Host}
	req.headers["Date"] = []string{time.Now().In(time.UTC).Format(time.RFC1123)}
	sign(s3.Auth, req.method, amazonEscape(req.signpath), req.params, req.headers)
	return nil
}

// run sends req and returns the http response from the server.
// If resp is not nil, the XML data contained in the response
// body will be unmarshalled on it.
func (s3 *S3) run(req *request, resp interface{}) (*http.Response, error) {
	if debug {
		log.Printf("Running S3 request: %#v", req)
	}

	u, err := req.url(false)
	if err != nil {
		return nil, err
	}

	hreq := http.Request{
		URL:        u,
		Method:     req.method,
		ProtoMajor: 1,
		ProtoMinor: 1,
		Close:      true,
		Header:     req.headers,
	}

	if v, ok := req.headers["Content-Length"]; ok {
		hreq.ContentLength, _ = strconv.ParseInt(v[0], 10, 64)
		delete(req.headers, "Content-Length")
	}
	if req.payload != nil {
		hreq.Body = ioutil.NopCloser(req.payload)
	}

	hresp, err := s3.HTTPClient().Do(&hreq)
	if err != nil {
		return nil, err
	}
	if debug {
		dump, _ := httputil.DumpResponse(hresp, true)
		log.Printf("} -> %s\n", dump)
	}
	if hresp.StatusCode != 200 && hresp.StatusCode != 204 {
		defer hresp.Body.Close()
		return nil, buildError(hresp)
	}
	if resp != nil {
		err = xml.NewDecoder(hresp.Body).Decode(resp)
		hresp.Body.Close()
	}
	return hresp, err
}

// Error represents an error in an operation with S3.
type Error struct {
	StatusCode int    // HTTP status code (200, 403, ...)
	Code       string // EC2 error code ("UnsupportedOperation", ...)
	Message    string // The human-oriented error message
	BucketName string
	RequestId  string
	HostId     string
}

func (e *Error) Error() string {
	return e.Message
}

func buildError(r *http.Response) error {
	if debug {
		log.Printf("got error (status code %v)", r.StatusCode)
		data, err := ioutil.ReadAll(r.Body)
		if err != nil {
			log.Printf("\tread error: %v", err)
		} else {
			log.Printf("\tdata:\n%s\n\n", data)
		}
		r.Body = ioutil.NopCloser(bytes.NewBuffer(data))
	}

	err := Error{}
	// TODO return error if Unmarshal fails?
	xml.NewDecoder(r.Body).Decode(&err)
	r.Body.Close()
	err.StatusCode = r.StatusCode
	if err.Message == "" {
		err.Message = r.Status
	}
	if debug {
		log.Printf("err: %#v\n", err)
	}
	return &err
}

func shouldRetry(err error) bool {
	if err == nil {
		return false
	}
	switch err {
	case io.ErrUnexpectedEOF, io.EOF:
		return true
	}
	switch e := err.(type) {
	case *net.DNSError:
		return true
	case *net.OpError:
		switch e.Op {
		case "read", "write":
			return true
		}
	case *Error:
		switch e.Code {
		case "InternalError", "NoSuchUpload", "NoSuchBucket":
			return true
		}
	}
	return false
}

func hasCode(err error, code string) bool {
	s3err, ok := err.(*Error)
	return ok && s3err.Code == code
}
