package oss

import (
	"net/url"
	"sort"

	. "gopkg.in/check.v1"
)

type OssTypeSuite struct{}

var _ = Suite(&OssTypeSuite{})

var (
	goStr     = "go go + go <> go"
	chnStr    = "试问闲情几许"
	goURLStr  = url.QueryEscape(goStr)
	chnURLStr = url.QueryEscape(chnStr)
)

func (s *OssTypeSuite) TestConvLifecycleRule(c *C) {
	r1 := BuildLifecycleRuleByDate("id1", "one", true, 2015, 11, 11)
	r2 := BuildLifecycleRuleByDays("id2", "two", false, 3)

	rs := convLifecycleRule([]LifecycleRule{r1})
	c.Assert(rs[0].ID, Equals, "id1")
	c.Assert(rs[0].Prefix, Equals, "one")
	c.Assert(rs[0].Status, Equals, "Enabled")
	c.Assert(rs[0].Expiration.Date, Equals, "2015-11-11T00:00:00.000Z")
	c.Assert(rs[0].Expiration.Days, Equals, 0)

	rs = convLifecycleRule([]LifecycleRule{r2})
	c.Assert(rs[0].ID, Equals, "id2")
	c.Assert(rs[0].Prefix, Equals, "two")
	c.Assert(rs[0].Status, Equals, "Disabled")
	c.Assert(rs[0].Expiration.Date, Equals, "")
	c.Assert(rs[0].Expiration.Days, Equals, 3)
}

func (s *OssTypeSuite) TestDecodeDeleteObjectsResult(c *C) {
	var res DeleteObjectsResult
	err := decodeDeleteObjectsResult(&res)
	c.Assert(err, IsNil)

	res.DeletedObjects = []string{""}
	err = decodeDeleteObjectsResult(&res)
	c.Assert(err, IsNil)
	c.Assert(res.DeletedObjects[0], Equals, "")

	res.DeletedObjects = []string{goURLStr, chnURLStr}
	err = decodeDeleteObjectsResult(&res)
	c.Assert(err, IsNil)
	c.Assert(res.DeletedObjects[0], Equals, goStr)
	c.Assert(res.DeletedObjects[1], Equals, chnStr)
}

func (s *OssTypeSuite) TestDecodeListObjectsResult(c *C) {
	var res ListObjectsResult
	err := decodeListObjectsResult(&res)
	c.Assert(err, IsNil)

	res = ListObjectsResult{}
	err = decodeListObjectsResult(&res)
	c.Assert(err, IsNil)

	res = ListObjectsResult{Prefix: goURLStr, Marker: goURLStr,
		Delimiter: goURLStr, NextMarker: goURLStr,
		Objects:        []ObjectProperties{{Key: chnURLStr}},
		CommonPrefixes: []string{chnURLStr}}

	err = decodeListObjectsResult(&res)
	c.Assert(err, IsNil)

	c.Assert(res.Prefix, Equals, goStr)
	c.Assert(res.Marker, Equals, goStr)
	c.Assert(res.Delimiter, Equals, goStr)
	c.Assert(res.NextMarker, Equals, goStr)
	c.Assert(res.Objects[0].Key, Equals, chnStr)
	c.Assert(res.CommonPrefixes[0], Equals, chnStr)
}

func (s *OssTypeSuite) TestDecodeListMultipartUploadResult(c *C) {
	res := ListMultipartUploadResult{}
	err := decodeListMultipartUploadResult(&res)
	c.Assert(err, IsNil)

	res = ListMultipartUploadResult{Prefix: goURLStr, KeyMarker: goURLStr,
		Delimiter: goURLStr, NextKeyMarker: goURLStr,
		Uploads: []UncompletedUpload{{Key: chnURLStr}}}

	err = decodeListMultipartUploadResult(&res)
	c.Assert(err, IsNil)

	c.Assert(res.Prefix, Equals, goStr)
	c.Assert(res.KeyMarker, Equals, goStr)
	c.Assert(res.Delimiter, Equals, goStr)
	c.Assert(res.NextKeyMarker, Equals, goStr)
	c.Assert(res.Uploads[0].Key, Equals, chnStr)
}

func (s *OssTypeSuite) TestSortUploadPart(c *C) {
	parts := []UploadPart{}

	sort.Sort(uploadParts(parts))
	c.Assert(len(parts), Equals, 0)

	parts = []UploadPart{
		{PartNumber: 5, ETag: "E5"},
		{PartNumber: 1, ETag: "E1"},
		{PartNumber: 4, ETag: "E4"},
		{PartNumber: 2, ETag: "E2"},
		{PartNumber: 3, ETag: "E3"},
	}

	sort.Sort(uploadParts(parts))

	c.Assert(parts[0].PartNumber, Equals, 1)
	c.Assert(parts[0].ETag, Equals, "E1")
	c.Assert(parts[1].PartNumber, Equals, 2)
	c.Assert(parts[1].ETag, Equals, "E2")
	c.Assert(parts[2].PartNumber, Equals, 3)
	c.Assert(parts[2].ETag, Equals, "E3")
	c.Assert(parts[3].PartNumber, Equals, 4)
	c.Assert(parts[3].ETag, Equals, "E4")
	c.Assert(parts[4].PartNumber, Equals, 5)
	c.Assert(parts[4].ETag, Equals, "E5")
}
