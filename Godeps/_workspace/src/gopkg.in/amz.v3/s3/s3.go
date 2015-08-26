//
// goamz - Go packages to interact with the Amazon Web Services.
//
//   https://wiki.ubuntu.com/goamz
//
// Copyright (c) 2011 Canonical Ltd.
//

package s3

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"gopkg.in/amz.v3/aws"
)

const debug = false

// The S3 type encapsulates operations with an S3 region.
type S3 struct {
	aws.Auth
	aws.Region
	Sign    aws.Signer
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

var (
	attempts        = defaultAttempts
	defaultAttempts = aws.AttemptStrategy{
		Min:   5,
		Total: 5 * time.Second,
		Delay: 200 * time.Millisecond,
	}
)

// RetryAttempts sets whether failing S3 requests may be retried to cope
// with eventual consistency or temporary failures. It should not be
// called while operations are in progress.
func RetryAttempts(retry bool) {
	if retry {
		attempts = defaultAttempts
	} else {
		attempts = aws.AttemptStrategy{}
	}
}

// New creates a new S3.
func New(auth aws.Auth, region aws.Region) *S3 {
	return &S3{auth, region, aws.SignV4Factory(region.Name, "s3"), 0}
}

// Bucket returns a Bucket with the given name.
func (s3 *S3) Bucket(name string) (*Bucket, error) {
	if strings.IndexAny(name, "/:@") >= 0 {
		return nil, fmt.Errorf("bad S3 bucket: %q", name)
	}
	if s3.Region.S3BucketEndpoint != "" || s3.Region.S3LowercaseBucket {
		name = strings.ToLower(name)
	}
	return &Bucket{s3, name}, nil
}

var createBucketConfiguration = `<CreateBucketConfiguration xmlns="http://s3.amazonaws.com/doc/2006-03-01/">
  <LocationConstraint>%s</LocationConstraint>
</CreateBucketConfiguration>`

// locationConstraint returns a *strings.Reader specifying a
// LocationConstraint if required for the region.
//
// See http://goo.gl/bh9Kq for details.
func (s3 *S3) locationConstraint() *strings.Reader {
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

// Put inserts an object into the S3 bucket.
//
// See http://goo.gl/FEBPD for details.
func (b *Bucket) Put(path string, data []byte, contType string, perm ACL) error {
	body := bytes.NewReader(data)
	return b.PutReader(path, body, int64(len(data)), contType, perm)
}

// PutBucket creates a new bucket.
//
// See http://goo.gl/ndjnR for details.
func (b *Bucket) PutBucket(perm ACL) error {
	body := b.locationConstraint()
	req, err := http.NewRequest("PUT", b.ResolveS3BucketEndpoint(b.Name), body)
	if err != nil {
		return err
	}
	req.Close = true

	addAmazonDateHeader(req.Header)
	req.Header.Add("x-amz-acl", string(perm))

	if err := b.S3.Sign(req, b.Auth); err != nil {
		return err
	}
	// Signing may read the request body.
	if _, err := body.Seek(0, 0); err != nil {
		return err
	}

	_, err = http.DefaultClient.Do(req)
	return err
}

// DelBucket removes an existing S3 bucket. All objects in the bucket must
// be removed before the bucket itself can be removed.
//
// See http://goo.gl/GoBrY for details.
func (b *Bucket) DelBucket() (err error) {

	req, err := http.NewRequest("DELETE", b.ResolveS3BucketEndpoint(b.Name), nil)
	if err != nil {
		return err
	}
	req.Close = true
	addAmazonDateHeader(req.Header)

	if err := b.S3.Sign(req, b.Auth); err != nil {
		return err
	}
	resp, err := requestRetryLoop(req, attempts)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// Get retrieves an object from an S3 bucket.
//
// See http://goo.gl/isCO7 for details.
func (b *Bucket) Get(path string) (data []byte, err error) {
	body, err := b.GetReader(path)
	if err != nil {
		return nil, err
	}
	defer body.Close()

	return ioutil.ReadAll(body)
}

// GetReader retrieves an object from an S3 bucket. It is the caller's
// responsibility to call Close on rc when finished reading.
func (b *Bucket) GetReader(path string) (rc io.ReadCloser, err error) {

	req, err := http.NewRequest("GET", b.Region.ResolveS3BucketEndpoint(b.Name), nil)
	if err != nil {
		return nil, err
	}
	req.Close = true
	req.URL.Path += path

	addAmazonDateHeader(req.Header)

	if err := b.S3.Sign(req, b.Auth); err != nil {
		return nil, err
	}

	resp, err := requestRetryLoop(req, attempts)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return nil, buildError(resp)
	}
	return resp.Body, nil
}

// PutReader inserts an object into the S3 bucket by consuming data
// from r until EOF. Passing in an io.ReadSeeker for r will optimize
// the memory usage.
func (b *Bucket) PutReader(path string, r io.Reader, length int64, contType string, perm ACL) error {
	return b.PutReaderWithHeader(path, r, length, contType, perm, http.Header{})
}

// PutReaderWithHeader inserts an object into the S3 bucket by
// consuming data from r until EOF. It also adds the headers provided
// to the request. Passing in an io.ReadSeeker for r will optimize the
// memory usage.
func (b *Bucket) PutReaderWithHeader(path string, r io.Reader, length int64, contType string, perm ACL, hdrs http.Header) error {

	// Convert the reader to a ReadSeeker so we can seek after
	// signing.
	seeker, ok := r.(io.ReadSeeker)
	if !ok {
		content, err := ioutil.ReadAll(r)
		if err != nil {
			return err
		}
		seeker = bytes.NewReader(content)
	}

	req, err := http.NewRequest("PUT", b.Region.ResolveS3BucketEndpoint(b.Name), seeker)
	if err != nil {
		return err
	}
	req.Header = hdrs
	req.Close = true
	req.URL.Path += path
	req.ContentLength = length

	req.Header.Add("Content-Type", contType)
	req.Header.Add("x-amz-acl", string(perm))
	addAmazonDateHeader(req.Header)

	// Determine the current offset.
	const seekFromPos = 1
	prevPos, err := seeker.Seek(0, seekFromPos)
	if err != nil {
		return err
	}

	if err := b.S3.Sign(req, b.Auth); err != nil {
		return err
	}
	// Signing may read the request body.
	if _, err := seeker.Seek(prevPos, 0); err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return buildError(resp) // closes body
	}

	resp.Body.Close()
	return nil
}

// Del removes an object from the S3 bucket.
//
// See http://goo.gl/APeTt for details.
func (b *Bucket) Del(path string) error {

	req, err := http.NewRequest("DELETE", b.ResolveS3BucketEndpoint(b.Name), nil)
	if err != nil {
		return err
	}
	req.Close = true
	req.URL.Path += path

	addAmazonDateHeader(req.Header)

	if err := b.S3.Sign(req, b.Auth); err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
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
func (b *Bucket) List(prefix, delim, marker string, max int) (*ListResp, error) {

	req, err := http.NewRequest("GET", b.ResolveS3BucketEndpoint(b.Name), nil)
	if err != nil {
		return nil, err
	}
	req.Close = true

	query := req.URL.Query()
	query.Add("prefix", prefix)
	query.Add("delimiter", delim)
	query.Add("marker", marker)
	if max != 0 {
		query.Add("max-keys", strconv.FormatInt(int64(max), 10))
	}
	req.URL.RawQuery = query.Encode()

	addAmazonDateHeader(req.Header)

	if err := b.S3.Sign(req, b.Auth); err != nil {
		return nil, err
	}

	resp, err := requestRetryLoop(req, attempts)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return nil, buildError(resp) // closes body
	}

	var result ListResp
	err = xml.NewDecoder(resp.Body).Decode(&result)
	resp.Body.Close()
	return &result, nil
}

// URL returns a non-signed URL that allows retriving the
// object at path. It only works if the object is publicly
// readable (see SignedURL).
func (b *Bucket) URL(path string) string {
	return b.ResolveS3BucketEndpoint(b.Name) + path
}

// SignedURL returns a URL which can be used to fetch objects without
// signing for the given duration.
func (b *Bucket) SignedURL(path string, expires time.Duration) (string, error) {
	req, err := http.NewRequest("GET", b.URL(path), nil)
	if err != nil {
		return "", err
	}
	req.Header.Add("date", time.Now().Format(aws.ISO8601BasicFormat))

	if err := aws.SignV4URL(req, b.Auth, b.Region.Name, "s3", expires); err != nil {
		return "", err
	}
	return req.URL.String(), nil
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

func (req *request) url() (*url.URL, error) {
	u, err := url.Parse(req.baseurl)
	if err != nil {
		return nil, fmt.Errorf("bad S3 endpoint URL %q: %v", req.baseurl, err)
	}
	u.RawQuery = req.params.Encode()
	u.Path = req.path
	return u, nil
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

// requestRetryLoop attempts to send the request until the given
// strategy says to stop.
func requestRetryLoop(req *http.Request, retryStrat aws.AttemptStrategy) (*http.Response, error) {

	for attempt := attempts.Start(); attempt.Next(); {

		if debug {
			log.Printf("Full URL (in loop): %v", req.URL)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			if shouldRetry(err) && attempt.HasNext() {
				continue
			}
			return nil, fmt.Errorf("making request: %v", err)
		}

		if debug {
			log.Printf("Full response (in loop): %v", resp)
		}

		return resp, nil
	}

	return nil, fmt.Errorf("could not complete the request within the specified retry attempts")
}

func addAmazonDateHeader(header http.Header) {
	header.Set("x-amz-date", time.Now().In(time.UTC).Format(aws.ISO8601BasicFormat))
}
