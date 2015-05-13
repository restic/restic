package s3_test

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"testing"

	"time"

	"github.com/mitchellh/goamz/aws"
	"github.com/mitchellh/goamz/s3"
	"github.com/mitchellh/goamz/testutil"
	. "github.com/motain/gocheck"
)

func Test(t *testing.T) {
	TestingT(t)
}

type S struct {
	s3 *s3.S3
}

var _ = Suite(&S{})

var testServer = testutil.NewHTTPServer()

func (s *S) SetUpSuite(c *C) {
	testServer.Start()
	auth := aws.Auth{"abc", "123", ""}
	s.s3 = s3.New(auth, aws.Region{Name: "faux-region-1", S3Endpoint: testServer.URL})
}

func (s *S) TearDownSuite(c *C) {
	s3.SetAttemptStrategy(nil)
}

func (s *S) SetUpTest(c *C) {
	attempts := aws.AttemptStrategy{
		Total: 300 * time.Millisecond,
		Delay: 100 * time.Millisecond,
	}
	s3.SetAttemptStrategy(&attempts)
}

func (s *S) TearDownTest(c *C) {
	testServer.Flush()
}

func (s *S) DisableRetries() {
	s3.SetAttemptStrategy(&aws.AttemptStrategy{})
}

// PutBucket docs: http://goo.gl/kBTCu

func (s *S) TestPutBucket(c *C) {
	testServer.Response(200, nil, "")

	b := s.s3.Bucket("bucket")
	err := b.PutBucket(s3.Private)
	c.Assert(err, IsNil)

	req := testServer.WaitRequest()
	c.Assert(req.Method, Equals, "PUT")
	c.Assert(req.URL.Path, Equals, "/bucket/")
	c.Assert(req.Header["Date"], Not(Equals), "")
}

// DeleteBucket docs: http://goo.gl/GoBrY

func (s *S) TestDelBucket(c *C) {
	testServer.Response(204, nil, "")

	b := s.s3.Bucket("bucket")
	err := b.DelBucket()
	c.Assert(err, IsNil)

	req := testServer.WaitRequest()
	c.Assert(req.Method, Equals, "DELETE")
	c.Assert(req.URL.Path, Equals, "/bucket/")
	c.Assert(req.Header["Date"], Not(Equals), "")
}

// ListBuckets: http://goo.gl/NqlyMN

func (s *S) TestListBuckets(c *C) {
	testServer.Response(200, nil, GetListBucketsDump)

	buckets, err := s.s3.ListBuckets()
	c.Assert(err, IsNil)
	c.Assert(len(buckets.Buckets), Equals, 2)
	c.Assert(buckets.Buckets[0].Name, Equals, "bucket1")
	c.Assert(buckets.Buckets[1].Name, Equals, "bucket2")

	req := testServer.WaitRequest()
	c.Assert(req.Method, Equals, "GET")
	c.Assert(req.URL.Path, Equals, "/")
}

// GetObject docs: http://goo.gl/isCO7

func (s *S) TestGet(c *C) {
	testServer.Response(200, nil, "content")

	b := s.s3.Bucket("bucket")
	data, err := b.Get("name")

	req := testServer.WaitRequest()
	c.Assert(req.Method, Equals, "GET")
	c.Assert(req.URL.Path, Equals, "/bucket/name")
	c.Assert(req.Header["Date"], Not(Equals), "")

	c.Assert(err, IsNil)
	c.Assert(string(data), Equals, "content")
}

func (s *S) TestHead(c *C) {
	testServer.Response(200, nil, "")
	b := s.s3.Bucket("bucket")
	resp, err := b.Head("name")
	req := testServer.WaitRequest()
	c.Assert(req.Method, Equals, "HEAD")
	c.Assert(req.URL.Path, Equals, "/bucket/name")
	c.Assert(req.Header["Date"], Not(Equals), "")

	c.Assert(err, IsNil)
	body, err := ioutil.ReadAll(resp.Body)
	c.Assert(err, IsNil)
	c.Assert(len(body), Equals, 0)
}

func (s *S) TestURL(c *C) {
	testServer.Response(200, nil, "content")

	b := s.s3.Bucket("bucket")
	url := b.URL("name")
	r, err := http.Get(url)
	c.Assert(err, IsNil)
	data, err := ioutil.ReadAll(r.Body)
	r.Body.Close()
	c.Assert(err, IsNil)
	c.Assert(string(data), Equals, "content")

	req := testServer.WaitRequest()
	c.Assert(req.Method, Equals, "GET")
	c.Assert(req.URL.Path, Equals, "/bucket/name")
}

func (s *S) TestGetReader(c *C) {
	testServer.Response(200, nil, "content")

	b := s.s3.Bucket("bucket")
	rc, err := b.GetReader("name")
	c.Assert(err, IsNil)
	data, err := ioutil.ReadAll(rc)
	rc.Close()
	c.Assert(err, IsNil)
	c.Assert(string(data), Equals, "content")

	req := testServer.WaitRequest()
	c.Assert(req.Method, Equals, "GET")
	c.Assert(req.URL.Path, Equals, "/bucket/name")
	c.Assert(req.Header["Date"], Not(Equals), "")
}

func (s *S) TestGetNotFound(c *C) {
	for i := 0; i < 10; i++ {
		testServer.Response(404, nil, GetObjectErrorDump)
	}

	b := s.s3.Bucket("non-existent-bucket")
	data, err := b.Get("non-existent")

	req := testServer.WaitRequest()
	c.Assert(req.Method, Equals, "GET")
	c.Assert(req.URL.Path, Equals, "/non-existent-bucket/non-existent")
	c.Assert(req.Header["Date"], Not(Equals), "")

	s3err, _ := err.(*s3.Error)
	c.Assert(s3err, NotNil)
	c.Assert(s3err.StatusCode, Equals, 404)
	c.Assert(s3err.BucketName, Equals, "non-existent-bucket")
	c.Assert(s3err.RequestId, Equals, "3F1B667FAD71C3D8")
	c.Assert(s3err.HostId, Equals, "L4ee/zrm1irFXY5F45fKXIRdOf9ktsKY/8TDVawuMK2jWRb1RF84i1uBzkdNqS5D")
	c.Assert(s3err.Code, Equals, "NoSuchBucket")
	c.Assert(s3err.Message, Equals, "The specified bucket does not exist")
	c.Assert(s3err.Error(), Equals, "The specified bucket does not exist")
	c.Assert(data, IsNil)
}

// PutObject docs: http://goo.gl/FEBPD

func (s *S) TestPutObject(c *C) {
	testServer.Response(200, nil, "")

	b := s.s3.Bucket("bucket")
	err := b.Put("name", []byte("content"), "content-type", s3.Private)
	c.Assert(err, IsNil)

	req := testServer.WaitRequest()
	c.Assert(req.Method, Equals, "PUT")
	c.Assert(req.URL.Path, Equals, "/bucket/name")
	c.Assert(req.Header["Date"], Not(DeepEquals), []string{""})
	c.Assert(req.Header["Content-Type"], DeepEquals, []string{"content-type"})
	c.Assert(req.Header["Content-Length"], DeepEquals, []string{"7"})
	//c.Assert(req.Header["Content-MD5"], DeepEquals, "...")
	c.Assert(req.Header["X-Amz-Acl"], DeepEquals, []string{"private"})
}

func (s *S) TestPutObjectHeader(c *C) {
	testServer.Response(200, nil, "")

	b := s.s3.Bucket("bucket")
	err := b.PutHeader(
		"name",
		[]byte("content"),
		map[string][]string{"Content-Type": {"content-type"}},
		s3.Private,
	)
	c.Assert(err, IsNil)

	req := testServer.WaitRequest()
	c.Assert(req.Method, Equals, "PUT")
	c.Assert(req.URL.Path, Equals, "/bucket/name")
	c.Assert(req.Header["Date"], Not(DeepEquals), []string{""})
	c.Assert(req.Header["Content-Type"], DeepEquals, []string{"content-type"})
	c.Assert(req.Header["Content-Length"], DeepEquals, []string{"7"})
	//c.Assert(req.Header["Content-MD5"], DeepEquals, "...")
	c.Assert(req.Header["X-Amz-Acl"], DeepEquals, []string{"private"})
}

func (s *S) TestPutReader(c *C) {
	testServer.Response(200, nil, "")

	b := s.s3.Bucket("bucket")
	buf := bytes.NewBufferString("content")
	err := b.PutReader("name", buf, int64(buf.Len()), "content-type", s3.Private)
	c.Assert(err, IsNil)

	req := testServer.WaitRequest()
	c.Assert(req.Method, Equals, "PUT")
	c.Assert(req.URL.Path, Equals, "/bucket/name")
	c.Assert(req.Header["Date"], Not(DeepEquals), []string{""})
	c.Assert(req.Header["Content-Type"], DeepEquals, []string{"content-type"})
	c.Assert(req.Header["Content-Length"], DeepEquals, []string{"7"})
	//c.Assert(req.Header["Content-MD5"], Equals, "...")
	c.Assert(req.Header["X-Amz-Acl"], DeepEquals, []string{"private"})
}

func (s *S) TestPutReaderHeader(c *C) {
	testServer.Response(200, nil, "")

	b := s.s3.Bucket("bucket")
	buf := bytes.NewBufferString("content")
	err := b.PutReaderHeader(
		"name",
		buf,
		int64(buf.Len()),
		map[string][]string{"Content-Type": {"content-type"}},
		s3.Private,
	)
	c.Assert(err, IsNil)

	req := testServer.WaitRequest()
	c.Assert(req.Method, Equals, "PUT")
	c.Assert(req.URL.Path, Equals, "/bucket/name")
	c.Assert(req.Header["Date"], Not(DeepEquals), []string{""})
	c.Assert(req.Header["Content-Type"], DeepEquals, []string{"content-type"})
	c.Assert(req.Header["Content-Length"], DeepEquals, []string{"7"})
	//c.Assert(req.Header["Content-MD5"], Equals, "...")
	c.Assert(req.Header["X-Amz-Acl"], DeepEquals, []string{"private"})
}

func (s *S) TestCopy(c *C) {
	testServer.Response(200, nil, "")

	b := s.s3.Bucket("bucket")
	err := b.Copy(
		"old/file",
		"new/file",
		s3.Private,
	)
	c.Assert(err, IsNil)

	req := testServer.WaitRequest()
	c.Assert(req.Method, Equals, "PUT")
	c.Assert(req.URL.Path, Equals, "/bucket/new/file")
	c.Assert(req.Header["X-Amz-Copy-Source"], DeepEquals, []string{"/bucket/old/file"})
	c.Assert(req.Header["X-Amz-Acl"], DeepEquals, []string{"private"})
}

func (s *S) TestPlusInURL(c *C) {
	testServer.Response(200, nil, "")

	b := s.s3.Bucket("bucket")
	err := b.Copy(
		"dir/old+f?le",
		"dir/new+f?le",
		s3.Private,
	)
	c.Assert(err, IsNil)

	req := testServer.WaitRequest()
	c.Assert(req.Method, Equals, "PUT")
	c.Assert(req.RequestURI, Equals, "/bucket/dir/new%2Bf%3Fle")
	c.Assert(req.Header["X-Amz-Copy-Source"], DeepEquals, []string{"/bucket/dir/old%2Bf%3Fle"})
	c.Assert(req.Header["X-Amz-Acl"], DeepEquals, []string{"private"})
}

// DelObject docs: http://goo.gl/APeTt

func (s *S) TestDelObject(c *C) {
	testServer.Response(200, nil, "")

	b := s.s3.Bucket("bucket")
	err := b.Del("name")
	c.Assert(err, IsNil)

	req := testServer.WaitRequest()
	c.Assert(req.Method, Equals, "DELETE")
	c.Assert(req.URL.Path, Equals, "/bucket/name")
	c.Assert(req.Header["Date"], Not(Equals), "")
}

// Delete Multiple Objects docs: http://goo.gl/WvA5sj

func (s *S) TestMultiDelObject(c *C) {
	testServer.Response(200, nil, "")

	b := s.s3.Bucket("bucket")
	err := b.MultiDel([]string{"a", "b"})
	c.Assert(err, IsNil)

	req := testServer.WaitRequest()
	c.Assert(req.Method, Equals, "POST")
	c.Assert(req.URL.Path, Equals, "/bucket/")
	c.Assert(req.RequestURI, Equals, "/bucket/?delete=")
	c.Assert(req.Header["Content-Md5"], DeepEquals, []string{"nos/vZNvjGs17xIyjEFlwQ=="})
	data, err := ioutil.ReadAll(req.Body)
	req.Body.Close()
	c.Assert(err, IsNil)
	c.Assert(string(data), Equals, "<Delete><Quiet>false</Quiet><Object><Key>a</Key></Object><Object><Key>b</Key></Object></Delete>")
}

// Bucket List Objects docs: http://goo.gl/YjQTc

func (s *S) TestList(c *C) {
	testServer.Response(200, nil, GetListResultDump1)

	b := s.s3.Bucket("quotes")

	data, err := b.List("N", "", "", 0)
	c.Assert(err, IsNil)

	req := testServer.WaitRequest()
	c.Assert(req.Method, Equals, "GET")
	c.Assert(req.URL.Path, Equals, "/quotes/")
	c.Assert(req.Header["Date"], Not(Equals), "")
	c.Assert(req.Form["prefix"], DeepEquals, []string{"N"})
	c.Assert(req.Form["delimiter"], DeepEquals, []string{""})
	c.Assert(req.Form["marker"], DeepEquals, []string{""})
	c.Assert(req.Form["max-keys"], DeepEquals, []string(nil))

	c.Assert(data.Name, Equals, "quotes")
	c.Assert(data.Prefix, Equals, "N")
	c.Assert(data.IsTruncated, Equals, false)
	c.Assert(len(data.Contents), Equals, 2)

	c.Assert(data.Contents[0].Key, Equals, "Nelson")
	c.Assert(data.Contents[0].LastModified, Equals, "2006-01-01T12:00:00.000Z")
	c.Assert(data.Contents[0].ETag, Equals, `"828ef3fdfa96f00ad9f27c383fc9ac7f"`)
	c.Assert(data.Contents[0].Size, Equals, int64(5))
	c.Assert(data.Contents[0].StorageClass, Equals, "STANDARD")
	c.Assert(data.Contents[0].Owner.ID, Equals, "bcaf161ca5fb16fd081034f")
	c.Assert(data.Contents[0].Owner.DisplayName, Equals, "webfile")

	c.Assert(data.Contents[1].Key, Equals, "Neo")
	c.Assert(data.Contents[1].LastModified, Equals, "2006-01-01T12:00:00.000Z")
	c.Assert(data.Contents[1].ETag, Equals, `"828ef3fdfa96f00ad9f27c383fc9ac7f"`)
	c.Assert(data.Contents[1].Size, Equals, int64(4))
	c.Assert(data.Contents[1].StorageClass, Equals, "STANDARD")
	c.Assert(data.Contents[1].Owner.ID, Equals, "bcaf1ffd86a5fb16fd081034f")
	c.Assert(data.Contents[1].Owner.DisplayName, Equals, "webfile")
}

func (s *S) TestListWithDelimiter(c *C) {
	testServer.Response(200, nil, GetListResultDump2)

	b := s.s3.Bucket("quotes")

	data, err := b.List("photos/2006/", "/", "some-marker", 1000)
	c.Assert(err, IsNil)

	req := testServer.WaitRequest()
	c.Assert(req.Method, Equals, "GET")
	c.Assert(req.URL.Path, Equals, "/quotes/")
	c.Assert(req.Header["Date"], Not(Equals), "")
	c.Assert(req.Form["prefix"], DeepEquals, []string{"photos/2006/"})
	c.Assert(req.Form["delimiter"], DeepEquals, []string{"/"})
	c.Assert(req.Form["marker"], DeepEquals, []string{"some-marker"})
	c.Assert(req.Form["max-keys"], DeepEquals, []string{"1000"})

	c.Assert(data.Name, Equals, "example-bucket")
	c.Assert(data.Prefix, Equals, "photos/2006/")
	c.Assert(data.Delimiter, Equals, "/")
	c.Assert(data.Marker, Equals, "some-marker")
	c.Assert(data.IsTruncated, Equals, false)
	c.Assert(len(data.Contents), Equals, 0)
	c.Assert(data.CommonPrefixes, DeepEquals, []string{"photos/2006/feb/", "photos/2006/jan/"})
}

func (s *S) TestGetKey(c *C) {
	testServer.Response(200, GetKeyHeaderDump, "")

	b := s.s3.Bucket("bucket")
	key, err := b.GetKey("name")

	req := testServer.WaitRequest()
	c.Assert(req.Method, Equals, "HEAD")
	c.Assert(req.URL.Path, Equals, "/bucket/name")
	c.Assert(req.Header["Date"], Not(Equals), "")

	c.Assert(err, IsNil)
	c.Assert(key.Key, Equals, "name")
	c.Assert(key.LastModified, Equals, GetKeyHeaderDump["Last-Modified"])
	c.Assert(key.Size, Equals, int64(434234))
	c.Assert(key.ETag, Equals, GetKeyHeaderDump["ETag"])
}

func (s *S) TestUnescapedColon(c *C) {
	b := s.s3.Bucket("bucket")
	u := b.URL("foo:bar")
	c.Assert(u, Equals, "http://localhost:4444/bucket/foo:bar")
}
