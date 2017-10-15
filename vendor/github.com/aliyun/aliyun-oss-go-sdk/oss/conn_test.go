package oss

import (
	"net/http"

	. "gopkg.in/check.v1"
)

type OssConnSuite struct{}

var _ = Suite(&OssConnSuite{})

func (s *OssConnSuite) TestURLMarker(c *C) {
	um := urlMaker{}
	um.Init("docs.github.com", true, false)
	c.Assert(um.Type, Equals, urlTypeCname)
	c.Assert(um.Scheme, Equals, "http")
	c.Assert(um.NetLoc, Equals, "docs.github.com")

	c.Assert(um.getURL("bucket", "object", "params").String(), Equals, "http://docs.github.com/object?params")
	c.Assert(um.getURL("bucket", "object", "").String(), Equals, "http://docs.github.com/object")
	c.Assert(um.getURL("", "object", "").String(), Equals, "http://docs.github.com/object")
	c.Assert(um.getResource("bucket", "object", "subres"), Equals, "/bucket/object?subres")
	c.Assert(um.getResource("bucket", "object", ""), Equals, "/bucket/object")
	c.Assert(um.getResource("", "object", ""), Equals, "/")

	um.Init("https://docs.github.com", true, false)
	c.Assert(um.Type, Equals, urlTypeCname)
	c.Assert(um.Scheme, Equals, "https")
	c.Assert(um.NetLoc, Equals, "docs.github.com")

	um.Init("http://docs.github.com", true, false)
	c.Assert(um.Type, Equals, urlTypeCname)
	c.Assert(um.Scheme, Equals, "http")
	c.Assert(um.NetLoc, Equals, "docs.github.com")

	um.Init("docs.github.com:8080", false, true)
	c.Assert(um.Type, Equals, urlTypeAliyun)
	c.Assert(um.Scheme, Equals, "http")
	c.Assert(um.NetLoc, Equals, "docs.github.com:8080")

	c.Assert(um.getURL("bucket", "object", "params").String(), Equals, "http://bucket.docs.github.com:8080/object?params")
	c.Assert(um.getURL("bucket", "object", "").String(), Equals, "http://bucket.docs.github.com:8080/object")
	c.Assert(um.getURL("", "object", "").String(), Equals, "http://docs.github.com:8080/")
	c.Assert(um.getResource("bucket", "object", "subres"), Equals, "/bucket/object?subres")
	c.Assert(um.getResource("bucket", "object", ""), Equals, "/bucket/object")
	c.Assert(um.getResource("", "object", ""), Equals, "/")

	um.Init("https://docs.github.com:8080", false, true)
	c.Assert(um.Type, Equals, urlTypeAliyun)
	c.Assert(um.Scheme, Equals, "https")
	c.Assert(um.NetLoc, Equals, "docs.github.com:8080")

	um.Init("127.0.0.1", false, true)
	c.Assert(um.Type, Equals, urlTypeIP)
	c.Assert(um.Scheme, Equals, "http")
	c.Assert(um.NetLoc, Equals, "127.0.0.1")

	um.Init("http://127.0.0.1", false, false)
	c.Assert(um.Type, Equals, urlTypeIP)
	c.Assert(um.Scheme, Equals, "http")
	c.Assert(um.NetLoc, Equals, "127.0.0.1")
	c.Assert(um.getURL("bucket", "object", "params").String(), Equals, "http://127.0.0.1/bucket/object?params")
	c.Assert(um.getURL("", "object", "params").String(), Equals, "http://127.0.0.1/?params")

	um.Init("https://127.0.0.1:8080", false, false)
	c.Assert(um.Type, Equals, urlTypeIP)
	c.Assert(um.Scheme, Equals, "https")
	c.Assert(um.NetLoc, Equals, "127.0.0.1:8080")
}

func (s *OssConnSuite) TestAuth(c *C) {
	endpoint := "https://github.com/"
	cfg := getDefaultOssConfig()
	um := urlMaker{}
	um.Init(endpoint, false, false)
	conn := Conn{cfg, &um, nil}
	uri := um.getURL("bucket", "object", "")
	req := &http.Request{
		Method:     "PUT",
		URL:        uri,
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header:     make(http.Header),
		Host:       uri.Host,
	}

	req.Header.Set("Content-Type", "text/html")
	req.Header.Set("Date", "Thu, 17 Nov 2005 18:49:58 GMT")
	req.Header.Set("Host", endpoint)
	req.Header.Set("X-OSS-Meta-Your", "your")
	req.Header.Set("X-OSS-Meta-Author", "foo@bar.com")
	req.Header.Set("X-OSS-Magic", "abracadabra")
	req.Header.Set("Content-Md5", "ODBGOERFMDMzQTczRUY3NUE3NzA5QzdFNUYzMDQxNEM=")

	conn.signHeader(req, um.getResource("bucket", "object", ""))
	testLogger.Println("AUTHORIZATION:", req.Header.Get(HTTPHeaderAuthorization))
}

func (s *OssConnSuite) TestConnToolFunc(c *C) {
	err := checkRespCode(202, []int{})
	c.Assert(err, NotNil)

	err = checkRespCode(202, []int{404})
	c.Assert(err, NotNil)

	err = checkRespCode(202, []int{202, 404})
	c.Assert(err, IsNil)

	srvErr, err := serviceErrFromXML([]byte(""), 312, "")
	c.Assert(err, NotNil)
	c.Assert(srvErr.StatusCode, Equals, 0)

	srvErr, err = serviceErrFromXML([]byte("ABC"), 312, "")
	c.Assert(err, NotNil)
	c.Assert(srvErr.StatusCode, Equals, 0)

	srvErr, err = serviceErrFromXML([]byte("<Error></Error>"), 312, "")
	c.Assert(err, IsNil)
	c.Assert(srvErr.StatusCode, Equals, 312)

	unexpect := UnexpectedStatusCodeError{[]int{200}, 202}
	c.Assert(len(unexpect.Error()) > 0, Equals, true)
	c.Assert(unexpect.Got(), Equals, 202)
}
