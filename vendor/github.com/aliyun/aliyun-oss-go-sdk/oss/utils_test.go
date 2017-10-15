package oss

import . "gopkg.in/check.v1"

type OssUtilsSuite struct{}

var _ = Suite(&OssUtilsSuite{})

func (s *OssUtilsSuite) TestUtilsTime(c *C) {
	c.Assert(GetNowSec() > 1448597674, Equals, true)
	c.Assert(GetNowNanoSec() > 1448597674000000000, Equals, true)
	c.Assert(len(GetNowGMT()), Equals, len("Fri, 27 Nov 2015 04:14:34 GMT"))
}

func (s *OssUtilsSuite) TestUtilsSplitFile(c *C) {
	localFile := "../sample/BingWallpaper-2015-11-07.jpg"

	// Num
	parts, err := SplitFileByPartNum(localFile, 4)
	c.Assert(err, IsNil)
	c.Assert(len(parts), Equals, 4)
	testLogger.Println("parts 4:", parts)
	for i, part := range parts {
		c.Assert(part.Number, Equals, i+1)
		c.Assert(part.Offset, Equals, int64(i*120512))
		c.Assert(part.Size, Equals, int64(120512))
	}

	parts, err = SplitFileByPartNum(localFile, 5)
	c.Assert(err, IsNil)
	c.Assert(len(parts), Equals, 5)
	testLogger.Println("parts 5:", parts)
	for i, part := range parts {
		c.Assert(part.Number, Equals, i+1)
		c.Assert(part.Offset, Equals, int64(i*96409))
	}

	_, err = SplitFileByPartNum(localFile, 10001)
	c.Assert(err, NotNil)

	_, err = SplitFileByPartNum(localFile, 0)
	c.Assert(err, NotNil)

	_, err = SplitFileByPartNum(localFile, -1)
	c.Assert(err, NotNil)

	_, err = SplitFileByPartNum("notexist", 1024)
	c.Assert(err, NotNil)

	// Size
	parts, err = SplitFileByPartSize(localFile, 120512)
	c.Assert(err, IsNil)
	c.Assert(len(parts), Equals, 4)
	testLogger.Println("parts 4:", parts)
	for i, part := range parts {
		c.Assert(part.Number, Equals, i+1)
		c.Assert(part.Offset, Equals, int64(i*120512))
		c.Assert(part.Size, Equals, int64(120512))
	}

	parts, err = SplitFileByPartSize(localFile, 96409)
	c.Assert(err, IsNil)
	c.Assert(len(parts), Equals, 6)
	testLogger.Println("parts 6:", parts)
	for i, part := range parts {
		c.Assert(part.Number, Equals, i+1)
		c.Assert(part.Offset, Equals, int64(i*96409))
	}

	_, err = SplitFileByPartSize(localFile, 0)
	c.Assert(err, NotNil)

	_, err = SplitFileByPartSize(localFile, -1)
	c.Assert(err, NotNil)

	_, err = SplitFileByPartSize(localFile, 10)
	c.Assert(err, NotNil)

	_, err = SplitFileByPartSize("noexist", 120512)
	c.Assert(err, NotNil)
}

func (s *OssUtilsSuite) TestUtilsFileExt(c *C) {
	c.Assert(TypeByExtension("test.txt"), Equals, "text/plain; charset=utf-8")
	c.Assert(TypeByExtension("test.jpg"), Equals, "image/jpeg")
	c.Assert(TypeByExtension("test.pdf"), Equals, "application/pdf")
	c.Assert(TypeByExtension("test"), Equals, "")
	c.Assert(TypeByExtension("/root/dir/test.txt"), Equals, "text/plain; charset=utf-8")
	c.Assert(TypeByExtension("root/dir/test.txt"), Equals, "text/plain; charset=utf-8")
	c.Assert(TypeByExtension("root\\dir\\test.txt"), Equals, "text/plain; charset=utf-8")
	c.Assert(TypeByExtension("D:\\work\\dir\\test.txt"), Equals, "text/plain; charset=utf-8")
}

func (s *OssUtilsSuite) TestGetPartEnd(c *C) {
	end := GetPartEnd(3, 10, 3)
	c.Assert(end, Equals, int64(5))

	end = GetPartEnd(9, 10, 3)
	c.Assert(end, Equals, int64(9))

	end = GetPartEnd(7, 10, 3)
	c.Assert(end, Equals, int64(9))
}

func (s *OssUtilsSuite) TestParseRange(c *C) {
	// InvalidRange bytes==M-N
	_, err := parseRange("bytes==M-N")
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "InvalidRange bytes==M-N")

	// InvalidRange ranges=M-N
	_, err = parseRange("ranges=M-N")
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "InvalidRange ranges=M-N")

	// InvalidRange ranges=M-N
	_, err = parseRange("bytes=M-N")
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "InvalidRange bytes=M-N")

	// InvalidRange ranges=M-
	_, err = parseRange("bytes=M-")
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "InvalidRange bytes=M-")

	// InvalidRange ranges=-N
	_, err = parseRange("bytes=-N")
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "InvalidRange bytes=-N")

	// InvalidRange ranges=-0
	_, err = parseRange("bytes=-0")
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "InvalidRange bytes=-0")

	// InvalidRange bytes=1-2-3
	_, err = parseRange("bytes=1-2-3")
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "InvalidRange bytes=1-2-3")

	// InvalidRange bytes=1-N
	_, err = parseRange("bytes=1-N")
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "InvalidRange bytes=1-N")

	// ranges=M-N
	ur, err := parseRange("bytes=1024-4096")
	c.Assert(err, IsNil)
	c.Assert(ur.start, Equals, (int64)(1024))
	c.Assert(ur.end, Equals, (int64)(4096))
	c.Assert(ur.hasStart, Equals, true)
	c.Assert(ur.hasEnd, Equals, true)

	// ranges=M-N,X-Y
	ur, err = parseRange("bytes=1024-4096,2048-4096")
	c.Assert(err, IsNil)
	c.Assert(ur.start, Equals, (int64)(1024))
	c.Assert(ur.end, Equals, (int64)(4096))
	c.Assert(ur.hasStart, Equals, true)
	c.Assert(ur.hasEnd, Equals, true)

	// ranges=M-
	ur, err = parseRange("bytes=1024-")
	c.Assert(err, IsNil)
	c.Assert(ur.start, Equals, (int64)(1024))
	c.Assert(ur.end, Equals, (int64)(0))
	c.Assert(ur.hasStart, Equals, true)
	c.Assert(ur.hasEnd, Equals, false)

	// ranges=-N
	ur, err = parseRange("bytes=-4096")
	c.Assert(err, IsNil)
	c.Assert(ur.start, Equals, (int64)(0))
	c.Assert(ur.end, Equals, (int64)(4096))
	c.Assert(ur.hasStart, Equals, false)
	c.Assert(ur.hasEnd, Equals, true)
}

func (s *OssUtilsSuite) TestAdjustRange(c *C) {
	// nil
	start, end := adjustRange(nil, 8192)
	c.Assert(start, Equals, (int64)(0))
	c.Assert(end, Equals, (int64)(8192))

	// 1024-4096
	ur := &unpackedRange{true, true, 1024, 4095}
	start, end = adjustRange(ur, 8192)
	c.Assert(start, Equals, (int64)(1024))
	c.Assert(end, Equals, (int64)(4096))

	// 1024-
	ur = &unpackedRange{true, false, 1024, 4096}
	start, end = adjustRange(ur, 8192)
	c.Assert(start, Equals, (int64)(1024))
	c.Assert(end, Equals, (int64)(8192))

	// -4096
	ur = &unpackedRange{false, true, 1024, 4096}
	start, end = adjustRange(ur, 8192)
	c.Assert(start, Equals, (int64)(4096))
	c.Assert(end, Equals, (int64)(8192))

	// Invalid range 4096-1024
	ur = &unpackedRange{true, true, 4096, 1024}
	start, end = adjustRange(ur, 8192)
	c.Assert(start, Equals, (int64)(0))
	c.Assert(end, Equals, (int64)(8192))

	// Invalid range -1-
	ur = &unpackedRange{true, false, -1, 0}
	start, end = adjustRange(ur, 8192)
	c.Assert(start, Equals, (int64)(0))
	c.Assert(end, Equals, (int64)(8192))

	// Invalid range -9999
	ur = &unpackedRange{false, true, 0, 9999}
	start, end = adjustRange(ur, 8192)
	c.Assert(start, Equals, (int64)(0))
	c.Assert(end, Equals, (int64)(8192))
}
