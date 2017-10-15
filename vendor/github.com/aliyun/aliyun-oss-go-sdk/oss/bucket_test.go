// bucket test

package oss

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/baiyubin/aliyun-sts-go-sdk/sts"

	. "gopkg.in/check.v1"
)

type OssBucketSuite struct {
	client        *Client
	bucket        *Bucket
	archiveBucket *Bucket
}

var _ = Suite(&OssBucketSuite{})

var (
	pastDate   = time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC)
	futureDate = time.Date(2049, time.January, 10, 23, 0, 0, 0, time.UTC)
)

// Run once when the suite starts running
func (s *OssBucketSuite) SetUpSuite(c *C) {
	client, err := New(endpoint, accessID, accessKey)
	c.Assert(err, IsNil)
	s.client = client

	s.client.CreateBucket(bucketName)

	err = s.client.CreateBucket(archiveBucketName, StorageClass(StorageArchive))
	c.Assert(err, IsNil)

	time.Sleep(5 * time.Second)

	bucket, err := s.client.Bucket(bucketName)
	c.Assert(err, IsNil)
	s.bucket = bucket

	archiveBucket, err := s.client.Bucket(archiveBucketName)
	c.Assert(err, IsNil)
	s.archiveBucket = archiveBucket

	testLogger.Println("test bucket started")
}

// Run before each test or benchmark starts running
func (s *OssBucketSuite) TearDownSuite(c *C) {
	for _, bucket := range []*Bucket{s.bucket, s.archiveBucket} {
		// Delete Multipart
		lmu, err := bucket.ListMultipartUploads()
		c.Assert(err, IsNil)

		for _, upload := range lmu.Uploads {
			imur := InitiateMultipartUploadResult{Bucket: bucketName, Key: upload.Key, UploadID: upload.UploadID}
			err = bucket.AbortMultipartUpload(imur)
			c.Assert(err, IsNil)
		}

		// Delete Objects
		lor, err := bucket.ListObjects()
		c.Assert(err, IsNil)

		for _, object := range lor.Objects {
			err = bucket.DeleteObject(object.Key)
			c.Assert(err, IsNil)
		}
	}

	testLogger.Println("test bucket completed")
}

// Run after each test or benchmark runs
func (s *OssBucketSuite) SetUpTest(c *C) {
	err := removeTempFiles("../oss", ".jpg")
	c.Assert(err, IsNil)
}

// Run once after all tests or benchmarks have finished running
func (s *OssBucketSuite) TearDownTest(c *C) {
	err := removeTempFiles("../oss", ".jpg")
	c.Assert(err, IsNil)

	err = removeTempFiles("../oss", ".txt")
	c.Assert(err, IsNil)

	err = removeTempFiles("../oss", ".temp")
	c.Assert(err, IsNil)

	err = removeTempFiles("../oss", ".txt1")
	c.Assert(err, IsNil)

	err = removeTempFiles("../oss", ".txt2")
	c.Assert(err, IsNil)
}

// TestPutObject
func (s *OssBucketSuite) TestPutObject(c *C) {
	objectName := objectNamePrefix + "tpo"
	objectValue := "大江东去，浪淘尽，千古风流人物。 故垒西边，人道是、三国周郎赤壁。 乱石穿空，惊涛拍岸，卷起千堆雪。 江山如画，一时多少豪杰。" +
		"遥想公谨当年，小乔初嫁了，雄姿英发。 羽扇纶巾，谈笑间、樯橹灰飞烟灭。故国神游，多情应笑我，早生华发，人生如梦，一尊还酹江月。"

	// string put
	err := s.bucket.PutObject(objectName, strings.NewReader(objectValue))
	c.Assert(err, IsNil)

	// Check
	body, err := s.bucket.GetObject(objectName)
	c.Assert(err, IsNil)
	str, err := readBody(body)
	c.Assert(err, IsNil)
	c.Assert(str, Equals, objectValue)

	acl, err := s.bucket.GetObjectACL(objectName)
	c.Assert(err, IsNil)
	testLogger.Println("aclRes:", acl)
	c.Assert(acl.ACL, Equals, "default")

	err = s.bucket.DeleteObject(objectName)
	c.Assert(err, IsNil)

	// bytes put
	err = s.bucket.PutObject(objectName, bytes.NewReader([]byte(objectValue)))
	c.Assert(err, IsNil)

	// Check
	body, err = s.bucket.GetObject(objectName)
	c.Assert(err, IsNil)
	str, err = readBody(body)
	c.Assert(err, IsNil)
	c.Assert(str, Equals, objectValue)

	err = s.bucket.DeleteObject(objectName)
	c.Assert(err, IsNil)

	// file put
	err = createFileAndWrite(objectName+".txt", []byte(objectValue))
	c.Assert(err, IsNil)
	fd, err := os.Open(objectName + ".txt")
	c.Assert(err, IsNil)

	err = s.bucket.PutObject(objectName, fd)
	c.Assert(err, IsNil)
	os.Remove(objectName + ".txt")

	// Check
	body, err = s.bucket.GetObject(objectName)
	c.Assert(err, IsNil)
	str, err = readBody(body)
	c.Assert(err, IsNil)
	c.Assert(str, Equals, objectValue)

	err = s.bucket.DeleteObject(objectName)
	c.Assert(err, IsNil)

	// Put with properties
	objectName = objectNamePrefix + "tpox"
	options := []Option{
		Expires(futureDate),
		ObjectACL(ACLPublicRead),
		Meta("myprop", "mypropval"),
	}
	err = s.bucket.PutObject(objectName, strings.NewReader(objectValue), options...)
	c.Assert(err, IsNil)

	// Check
	body, err = s.bucket.GetObject(objectName)
	c.Assert(err, IsNil)
	str, err = readBody(body)
	c.Assert(err, IsNil)
	c.Assert(str, Equals, objectValue)

	acl, err = s.bucket.GetObjectACL(objectName)
	c.Assert(err, IsNil)
	testLogger.Println("GetObjectACL:", acl)
	c.Assert(acl.ACL, Equals, string(ACLPublicRead))

	meta, err := s.bucket.GetObjectDetailedMeta(objectName)
	c.Assert(err, IsNil)
	testLogger.Println("GetObjectDetailedMeta:", meta)
	c.Assert(meta.Get("X-Oss-Meta-Myprop"), Equals, "mypropval")

	err = s.bucket.DeleteObject(objectName)
	c.Assert(err, IsNil)
}

func (s *OssBucketSuite) TestSignURL(c *C) {
	objectName := objectNamePrefix + randStr(5)
	objectValue := randStr(20)

	filePath := randLowStr(10)
	content := "复写object"
	createFile(filePath, content, c)

	notExistfilePath := randLowStr(10)
	os.Remove(notExistfilePath)

	// sign url for put
	str, err := s.bucket.SignURL(objectName, HTTPPut, 60)
	c.Assert(err, IsNil)
	c.Assert(strings.Contains(str, HTTPParamExpires+"="), Equals, true)
	c.Assert(strings.Contains(str, HTTPParamAccessKeyID+"="), Equals, true)
	c.Assert(strings.Contains(str, HTTPParamSignature+"="), Equals, true)

	// error put object with url
	err = s.bucket.PutObjectWithURL(str, strings.NewReader(objectValue), ContentType("image/tiff"))
	c.Assert(err, NotNil)
	c.Assert(err.(ServiceError).Code, Equals, "SignatureDoesNotMatch")

	err = s.bucket.PutObjectFromFileWithURL(str, filePath, ContentType("image/tiff"))
	c.Assert(err, NotNil)
	c.Assert(err.(ServiceError).Code, Equals, "SignatureDoesNotMatch")

	// put object with url
	err = s.bucket.PutObjectWithURL(str, strings.NewReader(objectValue))
	c.Assert(err, IsNil)

	acl, err := s.bucket.GetObjectACL(objectName)
	c.Assert(err, IsNil)
	c.Assert(acl.ACL, Equals, "default")

	// get object meta
	meta, err := s.bucket.GetObjectDetailedMeta(objectName)
	c.Assert(err, IsNil)
	c.Assert(meta.Get(HTTPHeaderContentType), Equals, "application/octet-stream")
	c.Assert(meta.Get("X-Oss-Meta-Myprop"), Equals, "")

	// sign url for get object
	str, err = s.bucket.SignURL(objectName, HTTPGet, 60)
	c.Assert(err, IsNil)
	c.Assert(strings.Contains(str, HTTPParamExpires+"="), Equals, true)
	c.Assert(strings.Contains(str, HTTPParamAccessKeyID+"="), Equals, true)
	c.Assert(strings.Contains(str, HTTPParamSignature+"="), Equals, true)

	// get object with url
	body, err := s.bucket.GetObjectWithURL(str)
	c.Assert(err, IsNil)
	str, err = readBody(body)
	c.Assert(err, IsNil)
	c.Assert(str, Equals, objectValue)

	// sign url for put with options
	options := []Option{
		ObjectACL(ACLPublicRead),
		Meta("myprop", "mypropval"),
		ContentType("image/tiff"),
		ResponseContentEncoding("deflate"),
	}
	str, err = s.bucket.SignURL(objectName, HTTPPut, 60, options...)
	c.Assert(err, IsNil)
	c.Assert(strings.Contains(str, HTTPParamExpires+"="), Equals, true)
	c.Assert(strings.Contains(str, HTTPParamAccessKeyID+"="), Equals, true)
	c.Assert(strings.Contains(str, HTTPParamSignature+"="), Equals, true)

	// put object with url from file
	// without option, error
	err = s.bucket.PutObjectWithURL(str, strings.NewReader(objectValue))
	c.Assert(err, NotNil)
	c.Assert(err.(ServiceError).Code, Equals, "SignatureDoesNotMatch")

	err = s.bucket.PutObjectFromFileWithURL(str, filePath)
	c.Assert(err, NotNil)
	c.Assert(err.(ServiceError).Code, Equals, "SignatureDoesNotMatch")

	// with option, error file
	err = s.bucket.PutObjectFromFileWithURL(str, notExistfilePath, options...)
	c.Assert(err, NotNil)

	// with option
	err = s.bucket.PutObjectFromFileWithURL(str, filePath, options...)
	c.Assert(err, IsNil)

	// get object meta
	meta, err = s.bucket.GetObjectDetailedMeta(objectName)
	c.Assert(err, IsNil)
	c.Assert(meta.Get("X-Oss-Meta-Myprop"), Equals, "mypropval")
	c.Assert(meta.Get(HTTPHeaderContentType), Equals, "image/tiff")

	acl, err = s.bucket.GetObjectACL(objectName)
	c.Assert(err, IsNil)
	c.Assert(acl.ACL, Equals, string(ACLPublicRead))

	// sign url for get object
	str, err = s.bucket.SignURL(objectName, HTTPGet, 60)
	c.Assert(err, IsNil)

	// get object to file with url
	newFile := randStr(10)
	err = s.bucket.GetObjectToFileWithURL(str, newFile)
	c.Assert(err, IsNil)
	eq, err := compareFiles(filePath, newFile)
	c.Assert(err, IsNil)
	c.Assert(eq, Equals, true)
	os.Remove(newFile)

	// get object to file error
	err = s.bucket.GetObjectToFileWithURL(str, newFile, options...)
	c.Assert(err, NotNil)
	c.Assert(err.(ServiceError).Code, Equals, "SignatureDoesNotMatch")
	_, err = os.Stat(newFile)
	c.Assert(err, NotNil)

	// get object error
	body, err = s.bucket.GetObjectWithURL(str, options...)
	c.Assert(err, NotNil)
	c.Assert(err.(ServiceError).Code, Equals, "SignatureDoesNotMatch")
	c.Assert(body, IsNil)

	// sign url for get object with options
	options = []Option{
		Expires(futureDate),
		ObjectACL(ACLPublicRead),
		Meta("myprop", "mypropval"),
		ContentType("image/tiff"),
		ResponseContentEncoding("deflate"),
	}
	str, err = s.bucket.SignURL(objectName, HTTPGet, 60, options...)
	c.Assert(err, IsNil)

	// get object to file with url and options
	err = s.bucket.GetObjectToFileWithURL(str, newFile, options...)
	c.Assert(err, IsNil)
	eq, err = compareFiles(filePath, newFile)
	c.Assert(err, IsNil)
	c.Assert(eq, Equals, true)
	os.Remove(newFile)

	// get object to file error
	err = s.bucket.GetObjectToFileWithURL(str, newFile)
	c.Assert(err, NotNil)
	c.Assert(err.(ServiceError).Code, Equals, "SignatureDoesNotMatch")
	_, err = os.Stat(newFile)
	c.Assert(err, NotNil)

	// get object error
	body, err = s.bucket.GetObjectWithURL(str)
	c.Assert(err, NotNil)
	c.Assert(err.(ServiceError).Code, Equals, "SignatureDoesNotMatch")
	c.Assert(body, IsNil)

	os.Remove(filePath)
	os.Remove(newFile)

	// sign url error
	str, err = s.bucket.SignURL(objectName, HTTPGet, -1)
	c.Assert(err, NotNil)

	err = s.bucket.DeleteObject(objectName)
	c.Assert(err, IsNil)

	// invalid url parse
	str = randStr(20)

	err = s.bucket.PutObjectWithURL(str, strings.NewReader(objectValue))
	c.Assert(err, NotNil)

	err = s.bucket.GetObjectToFileWithURL(str, newFile)
	c.Assert(err, NotNil)
}

func (s *OssBucketSuite) TestSignURLWithEscapedKey(c *C) {
	// key with '/'
	objectName := "zyimg/86/e8/653b5dc97bb0022051a84c632bc4"
	objectValue := "弃我去者，昨日之日不可留；乱我心者，今日之日多烦忧。长风万里送秋雁，对此可以酣高楼。蓬莱文章建安骨，中间小谢又清发。" +
		"俱怀逸兴壮思飞，欲上青天揽明月。抽刀断水水更流，举杯销愁愁更愁。人生在世不称意，明朝散发弄扁舟。"

	// sign url for put
	str, err := s.bucket.SignURL(objectName, HTTPPut, 60)
	c.Assert(err, IsNil)
	c.Assert(strings.Contains(str, HTTPParamExpires+"="), Equals, true)
	c.Assert(strings.Contains(str, HTTPParamAccessKeyID+"="), Equals, true)
	c.Assert(strings.Contains(str, HTTPParamSignature+"="), Equals, true)

	// put object with url
	err = s.bucket.PutObjectWithURL(str, strings.NewReader(objectValue))
	c.Assert(err, IsNil)

	// sign url for get object
	str, err = s.bucket.SignURL(objectName, HTTPGet, 60)
	c.Assert(err, IsNil)
	c.Assert(strings.Contains(str, HTTPParamExpires+"="), Equals, true)
	c.Assert(strings.Contains(str, HTTPParamAccessKeyID+"="), Equals, true)
	c.Assert(strings.Contains(str, HTTPParamSignature+"="), Equals, true)

	// get object with url
	body, err := s.bucket.GetObjectWithURL(str)
	c.Assert(err, IsNil)
	str, err = readBody(body)
	c.Assert(err, IsNil)
	c.Assert(str, Equals, objectValue)

	// key with escaped chars
	objectName = "<>[]()`?.,!@#$%^&'/*-_=+~:;"

	// sign url for put
	str, err = s.bucket.SignURL(objectName, HTTPPut, 60)
	c.Assert(err, IsNil)
	c.Assert(strings.Contains(str, HTTPParamExpires+"="), Equals, true)
	c.Assert(strings.Contains(str, HTTPParamAccessKeyID+"="), Equals, true)
	c.Assert(strings.Contains(str, HTTPParamSignature+"="), Equals, true)

	// put object with url
	err = s.bucket.PutObjectWithURL(str, strings.NewReader(objectValue))
	c.Assert(err, IsNil)

	// sign url for get object
	str, err = s.bucket.SignURL(objectName, HTTPGet, 60)
	c.Assert(err, IsNil)
	c.Assert(strings.Contains(str, HTTPParamExpires+"="), Equals, true)
	c.Assert(strings.Contains(str, HTTPParamAccessKeyID+"="), Equals, true)
	c.Assert(strings.Contains(str, HTTPParamSignature+"="), Equals, true)

	// get object with url
	body, err = s.bucket.GetObjectWithURL(str)
	c.Assert(err, IsNil)
	str, err = readBody(body)
	c.Assert(err, IsNil)
	c.Assert(str, Equals, objectValue)

	// key with Chinese chars
	objectName = "风吹柳花满店香，吴姬压酒劝客尝。金陵子弟来相送，欲行不行各尽觞。请君试问东流水，别意与之谁短长。"

	// sign url for put
	str, err = s.bucket.SignURL(objectName, HTTPPut, 60)
	c.Assert(err, IsNil)
	c.Assert(strings.Contains(str, HTTPParamExpires+"="), Equals, true)
	c.Assert(strings.Contains(str, HTTPParamAccessKeyID+"="), Equals, true)
	c.Assert(strings.Contains(str, HTTPParamSignature+"="), Equals, true)

	// put object with url
	err = s.bucket.PutObjectWithURL(str, strings.NewReader(objectValue))
	c.Assert(err, IsNil)

	// sign url for get object
	str, err = s.bucket.SignURL(objectName, HTTPGet, 60)
	c.Assert(err, IsNil)
	c.Assert(strings.Contains(str, HTTPParamExpires+"="), Equals, true)
	c.Assert(strings.Contains(str, HTTPParamAccessKeyID+"="), Equals, true)
	c.Assert(strings.Contains(str, HTTPParamSignature+"="), Equals, true)

	// get object with url
	body, err = s.bucket.GetObjectWithURL(str)
	c.Assert(err, IsNil)
	str, err = readBody(body)
	c.Assert(err, IsNil)
	c.Assert(str, Equals, objectValue)

	// key
	objectName = "test/此情无计可消除/才下眉头/却上 心头/。，；：‘’“”？（）『』【】《》！@#￥%……&×/test+ =-_*&^%$#@!`~[]{}()<>|\\/?.,;.txt"

	// sign url for put
	str, err = s.bucket.SignURL(objectName, HTTPPut, 60)
	c.Assert(err, IsNil)

	// put object with url
	err = s.bucket.PutObjectWithURL(str, strings.NewReader(objectValue))
	c.Assert(err, IsNil)

	// sign url for get object
	str, err = s.bucket.SignURL(objectName, HTTPGet, 60)
	c.Assert(err, IsNil)

	// get object with url
	body, err = s.bucket.GetObjectWithURL(str)
	c.Assert(err, IsNil)
	str, err = readBody(body)
	c.Assert(err, IsNil)
	c.Assert(str, Equals, objectValue)

	// put object
	err = s.bucket.PutObject(objectName, bytes.NewReader([]byte(objectValue)))
	c.Assert(err, IsNil)

	// get object
	body, err = s.bucket.GetObject(objectName)
	c.Assert(err, IsNil)
	str, err = readBody(body)
	c.Assert(err, IsNil)
	c.Assert(str, Equals, objectValue)

	// delete object
	err = s.bucket.DeleteObject(objectName)
	c.Assert(err, IsNil)
}

func (s *OssBucketSuite) TestSignURLWithEscapedKeyAndPorxy(c *C) {
	// key with '/'
	objectName := "zyimg/86/e8/653b5dc97bb0022051a84c632bc4"
	objectValue := "弃我去者，昨日之日不可留；乱我心者，今日之日多烦忧。长风万里送秋雁，对此可以酣高楼。蓬莱文章建安骨，中间小谢又清发。" +
		"俱怀逸兴壮思飞，欲上青天揽明月。抽刀断水水更流，举杯销愁愁更愁。人生在世不称意，明朝散发弄扁舟。"

	client, err := New(endpoint, accessID, accessKey, AuthProxy(proxyHost, proxyUser, proxyPasswd))
	bucket, err := client.Bucket(bucketName)

	// sign url for put
	str, err := bucket.SignURL(objectName, HTTPPut, 60)
	c.Assert(err, IsNil)
	c.Assert(strings.Contains(str, HTTPParamExpires+"="), Equals, true)
	c.Assert(strings.Contains(str, HTTPParamAccessKeyID+"="), Equals, true)
	c.Assert(strings.Contains(str, HTTPParamSignature+"="), Equals, true)

	// put object with url
	err = bucket.PutObjectWithURL(str, strings.NewReader(objectValue))
	c.Assert(err, IsNil)

	// sign url for get object
	str, err = bucket.SignURL(objectName, HTTPGet, 60)
	c.Assert(err, IsNil)
	c.Assert(strings.Contains(str, HTTPParamExpires+"="), Equals, true)
	c.Assert(strings.Contains(str, HTTPParamAccessKeyID+"="), Equals, true)
	c.Assert(strings.Contains(str, HTTPParamSignature+"="), Equals, true)

	// get object with url
	body, err := bucket.GetObjectWithURL(str)
	c.Assert(err, IsNil)
	str, err = readBody(body)
	c.Assert(err, IsNil)
	c.Assert(str, Equals, objectValue)

	// key with Chinese chars
	objectName = "test/此情无计可消除/才下眉头/却上 心头/。，；：‘’“”？（）『』【】《》！@#￥%……&×/test+ =-_*&^%$#@!`~[]{}()<>|\\/?.,;.txt"

	// sign url for put
	str, err = bucket.SignURL(objectName, HTTPPut, 60)
	c.Assert(err, IsNil)

	// put object with url
	err = bucket.PutObjectWithURL(str, strings.NewReader(objectValue))
	c.Assert(err, IsNil)

	// sign url for get object
	str, err = bucket.SignURL(objectName, HTTPGet, 60)
	c.Assert(err, IsNil)

	// get object with url
	body, err = bucket.GetObjectWithURL(str)
	c.Assert(err, IsNil)
	str, err = readBody(body)
	c.Assert(err, IsNil)
	c.Assert(str, Equals, objectValue)

	// put object
	err = bucket.PutObject(objectName, bytes.NewReader([]byte(objectValue)))
	c.Assert(err, IsNil)

	// get object
	body, err = bucket.GetObject(objectName)
	c.Assert(err, IsNil)
	str, err = readBody(body)
	c.Assert(err, IsNil)
	c.Assert(str, Equals, objectValue)

	// delete object
	err = bucket.DeleteObject(objectName)
	c.Assert(err, IsNil)
}

// TestPutObjectType
func (s *OssBucketSuite) TestPutObjectType(c *C) {
	objectName := objectNamePrefix + "tptt"
	objectValue := "乱石穿空，惊涛拍岸，卷起千堆雪。 江山如画，一时多少豪杰。"

	// Put
	err := s.bucket.PutObject(objectName, strings.NewReader(objectValue))
	c.Assert(err, IsNil)

	// Check
	time.Sleep(time.Second)
	body, err := s.bucket.GetObject(objectName)
	c.Assert(err, IsNil)
	str, err := readBody(body)
	c.Assert(err, IsNil)
	c.Assert(str, Equals, objectValue)

	meta, err := s.bucket.GetObjectDetailedMeta(objectName)
	c.Assert(err, IsNil)
	c.Assert(meta.Get("Content-Type"), Equals, "application/octet-stream")

	err = s.bucket.DeleteObject(objectName)
	c.Assert(err, IsNil)

	// Put
	err = s.bucket.PutObject(objectName+".txt", strings.NewReader(objectValue))
	c.Assert(err, IsNil)

	meta, err = s.bucket.GetObjectDetailedMeta(objectName + ".txt")
	c.Assert(err, IsNil)
	c.Assert(meta.Get("Content-Type"), Equals, "text/plain; charset=utf-8")

	err = s.bucket.DeleteObject(objectName + ".txt")
	c.Assert(err, IsNil)

	// Put
	err = s.bucket.PutObject(objectName+".apk", strings.NewReader(objectValue))
	c.Assert(err, IsNil)

	meta, err = s.bucket.GetObjectDetailedMeta(objectName + ".apk")
	c.Assert(err, IsNil)
	c.Assert(meta.Get("Content-Type"), Equals, "application/vnd.android.package-archive")

	err = s.bucket.DeleteObject(objectName + ".txt")
	c.Assert(err, IsNil)
}

// TestPutObject
func (s *OssBucketSuite) TestPutObjectKeyChars(c *C) {
	objectName := objectNamePrefix + "tpokc"
	objectValue := "白日依山尽，黄河入海流。欲穷千里目，更上一层楼。"

	// Put
	objectKey := objectName + "十步杀一人，千里不留行。事了拂衣去，深藏身与名"
	err := s.bucket.PutObject(objectKey, strings.NewReader(objectValue))
	c.Assert(err, IsNil)

	// Check
	body, err := s.bucket.GetObject(objectKey)
	c.Assert(err, IsNil)
	str, err := readBody(body)
	c.Assert(err, IsNil)
	c.Assert(str, Equals, objectValue)

	err = s.bucket.DeleteObject(objectKey)
	c.Assert(err, IsNil)

	// Put
	objectKey = objectName + "ごきげん如何ですかおれの顔をよく拝んでおけ"
	err = s.bucket.PutObject(objectKey, strings.NewReader(objectValue))
	c.Assert(err, IsNil)

	// Check
	body, err = s.bucket.GetObject(objectKey)
	c.Assert(err, IsNil)
	str, err = readBody(body)
	c.Assert(err, IsNil)
	c.Assert(str, Equals, objectValue)

	err = s.bucket.DeleteObject(objectKey)
	c.Assert(err, IsNil)

	// Put
	objectKey = objectName + "~!@#$%^&*()_-+=|\\[]{}<>,./?"
	err = s.bucket.PutObject(objectKey, strings.NewReader(objectValue))
	c.Assert(err, IsNil)

	// Check
	body, err = s.bucket.GetObject(objectKey)
	c.Assert(err, IsNil)
	str, err = readBody(body)
	c.Assert(err, IsNil)
	c.Assert(str, Equals, objectValue)

	err = s.bucket.DeleteObject(objectKey)
	c.Assert(err, IsNil)

	// Put
	objectKey = "go/中国 日本 +-#&=*"
	err = s.bucket.PutObject(objectKey, strings.NewReader(objectValue))
	c.Assert(err, IsNil)

	// Check
	body, err = s.bucket.GetObject(objectKey)
	c.Assert(err, IsNil)
	str, err = readBody(body)
	c.Assert(err, IsNil)
	c.Assert(str, Equals, objectValue)

	err = s.bucket.DeleteObject(objectKey)
	c.Assert(err, IsNil)
}

// TestPutObjectNegative
func (s *OssBucketSuite) TestPutObjectNegative(c *C) {
	objectName := objectNamePrefix + "tpon"
	objectValue := "大江东去，浪淘尽，千古风流人物。 "

	// Put
	objectName = objectNamePrefix + "tpox"
	err := s.bucket.PutObject(objectName, strings.NewReader(objectValue),
		Meta("meta-my", "myprop"))
	c.Assert(err, IsNil)

	// Check meta
	body, err := s.bucket.GetObject(objectName)
	c.Assert(err, IsNil)
	str, err := readBody(body)
	c.Assert(err, IsNil)
	c.Assert(str, Equals, objectValue)

	meta, err := s.bucket.GetObjectDetailedMeta(objectName)
	c.Assert(err, IsNil)
	c.Assert(meta.Get("X-Oss-Meta-My"), Not(Equals), "myprop")
	c.Assert(meta.Get("X-Oss-Meta-My"), Equals, "")

	err = s.bucket.DeleteObject(objectName)
	c.Assert(err, IsNil)

	// invalid option
	err = s.bucket.PutObject(objectName, strings.NewReader(objectValue),
		IfModifiedSince(pastDate))
	c.Assert(err, NotNil)

	err = s.bucket.PutObjectFromFile(objectName, "bucket.go", IfModifiedSince(pastDate))
	c.Assert(err, NotNil)

	err = s.bucket.PutObjectFromFile(objectName, "/tmp/xxx")
	c.Assert(err, NotNil)
}

// TestPutObjectFromFile
func (s *OssBucketSuite) TestPutObjectFromFile(c *C) {
	objectName := objectNamePrefix + "tpoff"
	localFile := "../sample/BingWallpaper-2015-11-07.jpg"
	newFile := "newpic11.jpg"

	// Put
	err := s.bucket.PutObjectFromFile(objectName, localFile)
	c.Assert(err, IsNil)

	// Check
	err = s.bucket.GetObjectToFile(objectName, newFile)
	c.Assert(err, IsNil)
	eq, err := compareFiles(localFile, newFile)
	c.Assert(err, IsNil)
	c.Assert(eq, Equals, true)

	acl, err := s.bucket.GetObjectACL(objectName)
	c.Assert(err, IsNil)
	testLogger.Println("aclRes:", acl)
	c.Assert(acl.ACL, Equals, "default")

	err = s.bucket.DeleteObject(objectName)
	c.Assert(err, IsNil)

	// Put with properties
	options := []Option{
		Expires(futureDate),
		ObjectACL(ACLPublicRead),
		Meta("myprop", "mypropval"),
	}
	os.Remove(newFile)
	err = s.bucket.PutObjectFromFile(objectName, localFile, options...)
	c.Assert(err, IsNil)

	// Check
	err = s.bucket.GetObjectToFile(objectName, newFile)
	c.Assert(err, IsNil)
	eq, err = compareFiles(localFile, newFile)
	c.Assert(err, IsNil)
	c.Assert(eq, Equals, true)

	acl, err = s.bucket.GetObjectACL(objectName)
	c.Assert(err, IsNil)
	testLogger.Println("GetObjectACL:", acl)
	c.Assert(acl.ACL, Equals, string(ACLPublicRead))

	meta, err := s.bucket.GetObjectDetailedMeta(objectName)
	c.Assert(err, IsNil)
	testLogger.Println("GetObjectDetailedMeta:", meta)
	c.Assert(meta.Get("X-Oss-Meta-Myprop"), Equals, "mypropval")

	err = s.bucket.DeleteObject(objectName)
	c.Assert(err, IsNil)
	os.Remove(newFile)
}

// TestPutObjectFromFile
func (s *OssBucketSuite) TestPutObjectFromFileType(c *C) {
	objectName := objectNamePrefix + "tpoffwm"
	localFile := "../sample/BingWallpaper-2015-11-07.jpg"
	newFile := "newpic11.jpg"

	// Put
	err := s.bucket.PutObjectFromFile(objectName, localFile)
	c.Assert(err, IsNil)

	// Check
	err = s.bucket.GetObjectToFile(objectName, newFile)
	c.Assert(err, IsNil)
	eq, err := compareFiles(localFile, newFile)
	c.Assert(err, IsNil)
	c.Assert(eq, Equals, true)

	meta, err := s.bucket.GetObjectDetailedMeta(objectName)
	c.Assert(err, IsNil)
	c.Assert(meta.Get("Content-Type"), Equals, "image/jpeg")

	err = s.bucket.DeleteObject(objectName)
	c.Assert(err, IsNil)
	os.Remove(newFile)
}

// TestGetObject
func (s *OssBucketSuite) TestGetObject(c *C) {
	objectName := objectNamePrefix + "tgo"
	objectValue := "长忆观潮，满郭人争江上望。来疑沧海尽成空，万面鼓声中。弄潮儿向涛头立，手把红旗旗不湿。别来几向梦中看，梦觉尚心寒。"

	// Put
	err := s.bucket.PutObject(objectName, strings.NewReader(objectValue))
	c.Assert(err, IsNil)

	// Check
	body, err := s.bucket.GetObject(objectName)
	c.Assert(err, IsNil)
	data, err := ioutil.ReadAll(body)
	body.Close()
	str := string(data)
	c.Assert(str, Equals, objectValue)
	testLogger.Println("GetObjec:", str)

	// Range
	var subObjectValue = string(([]byte(objectValue))[15:36])
	body, err = s.bucket.GetObject(objectName, Range(15, 35))
	c.Assert(err, IsNil)
	data, err = ioutil.ReadAll(body)
	body.Close()
	str = string(data)
	c.Assert(str, Equals, subObjectValue)
	testLogger.Println("GetObject:", str, ",", subObjectValue)

	// If-Modified-Since
	_, err = s.bucket.GetObject(objectName, IfModifiedSince(futureDate))
	c.Assert(err, NotNil)

	// If-Unmodified-Since
	body, err = s.bucket.GetObject(objectName, IfUnmodifiedSince(futureDate))
	c.Assert(err, IsNil)
	data, err = ioutil.ReadAll(body)
	body.Close()
	c.Assert(string(data), Equals, objectValue)

	meta, err := s.bucket.GetObjectDetailedMeta(objectName)
	c.Assert(err, IsNil)

	// If-Match
	body, err = s.bucket.GetObject(objectName, IfMatch(meta.Get("Etag")))
	c.Assert(err, IsNil)
	data, err = ioutil.ReadAll(body)
	body.Close()
	c.Assert(string(data), Equals, objectValue)

	// If-None-Match
	_, err = s.bucket.GetObject(objectName, IfNoneMatch(meta.Get("Etag")))
	c.Assert(err, NotNil)

	err = s.bucket.DeleteObject(objectName)
	c.Assert(err, IsNil)
}

// TestGetObjectNegative
func (s *OssBucketSuite) TestGetObjectToWriterNegative(c *C) {
	objectName := objectNamePrefix + "tgotwn"
	objectValue := "长忆观潮，满郭人争江上望。"

	// object not exist
	_, err := s.bucket.GetObject("NotExist")
	c.Assert(err, NotNil)

	// constraint invalid
	err = s.bucket.PutObject(objectName, strings.NewReader(objectValue))
	c.Assert(err, IsNil)

	// out of range
	_, err = s.bucket.GetObject(objectName, Range(15, 1000))
	c.Assert(err, IsNil)

	// no exist
	err = s.bucket.GetObjectToFile(objectName, "/root/123abc9874")
	c.Assert(err, NotNil)

	// invalid option
	_, err = s.bucket.GetObject(objectName, ACL(ACLPublicRead))
	c.Assert(err, IsNil)

	err = s.bucket.GetObjectToFile(objectName, "newpic15.jpg", ACL(ACLPublicRead))
	c.Assert(err, IsNil)

	err = s.bucket.DeleteObject(objectName)
	c.Assert(err, IsNil)
}

// TestGetObjectToFile
func (s *OssBucketSuite) TestGetObjectToFile(c *C) {
	objectName := objectNamePrefix + "tgotf"
	objectValue := "江南好，风景旧曾谙；日出江花红胜火，春来江水绿如蓝。能不忆江南？江南忆，最忆是杭州；山寺月中寻桂子，郡亭枕上看潮头。何日更重游！"
	newFile := "newpic15.jpg"

	// Put
	var val = []byte(objectValue)
	err := s.bucket.PutObject(objectName, strings.NewReader(objectValue))
	c.Assert(err, IsNil)

	// Check
	err = s.bucket.GetObjectToFile(objectName, newFile)
	c.Assert(err, IsNil)
	eq, err := compareFileData(newFile, val)
	c.Assert(err, IsNil)
	c.Assert(eq, Equals, true)
	os.Remove(newFile)

	// Range
	err = s.bucket.GetObjectToFile(objectName, newFile, Range(15, 35))
	c.Assert(err, IsNil)
	eq, err = compareFileData(newFile, val[15:36])
	c.Assert(err, IsNil)
	c.Assert(eq, Equals, true)
	os.Remove(newFile)

	err = s.bucket.GetObjectToFile(objectName, newFile, NormalizedRange("15-35"))
	c.Assert(err, IsNil)
	eq, err = compareFileData(newFile, val[15:36])
	c.Assert(err, IsNil)
	c.Assert(eq, Equals, true)
	os.Remove(newFile)

	err = s.bucket.GetObjectToFile(objectName, newFile, NormalizedRange("15-"))
	c.Assert(err, IsNil)
	eq, err = compareFileData(newFile, val[15:])
	c.Assert(err, IsNil)
	c.Assert(eq, Equals, true)
	os.Remove(newFile)

	err = s.bucket.GetObjectToFile(objectName, newFile, NormalizedRange("-10"))
	c.Assert(err, IsNil)
	eq, err = compareFileData(newFile, val[(len(val)-10):len(val)])
	c.Assert(err, IsNil)
	c.Assert(eq, Equals, true)
	os.Remove(newFile)

	// If-Modified-Since
	err = s.bucket.GetObjectToFile(objectName, newFile, IfModifiedSince(futureDate))
	c.Assert(err, NotNil)

	// If-Unmodified-Since
	err = s.bucket.GetObjectToFile(objectName, newFile, IfUnmodifiedSince(futureDate))
	c.Assert(err, IsNil)
	eq, err = compareFileData(newFile, val)
	c.Assert(err, IsNil)
	c.Assert(eq, Equals, true)
	os.Remove(newFile)

	meta, err := s.bucket.GetObjectDetailedMeta(objectName)
	c.Assert(err, IsNil)
	testLogger.Println("GetObjectDetailedMeta:", meta)

	// If-Match
	err = s.bucket.GetObjectToFile(objectName, newFile, IfMatch(meta.Get("Etag")))
	c.Assert(err, IsNil)
	eq, err = compareFileData(newFile, val)
	c.Assert(err, IsNil)
	c.Assert(eq, Equals, true)
	os.Remove(newFile)

	// If-None-Match
	err = s.bucket.GetObjectToFile(objectName, newFile, IfNoneMatch(meta.Get("Etag")))
	c.Assert(err, NotNil)

	err = s.bucket.DeleteObject(objectName)
	c.Assert(err, IsNil)
}

// TestListObjects
func (s *OssBucketSuite) TestListObjects(c *C) {
	objectName := objectNamePrefix + "tlo"

	// list empty bucket
	lor, err := s.bucket.ListObjects()
	c.Assert(err, IsNil)
	left := len(lor.Objects)

	// Put three object
	err = s.bucket.PutObject(objectName+"1", strings.NewReader(""))
	c.Assert(err, IsNil)
	err = s.bucket.PutObject(objectName+"2", strings.NewReader(""))
	c.Assert(err, IsNil)
	err = s.bucket.PutObject(objectName+"3", strings.NewReader(""))
	c.Assert(err, IsNil)

	// list
	lor, err = s.bucket.ListObjects()
	c.Assert(err, IsNil)
	c.Assert(len(lor.Objects), Equals, left+3)

	// list with prefix
	lor, err = s.bucket.ListObjects(Prefix(objectName + "2"))
	c.Assert(err, IsNil)
	c.Assert(len(lor.Objects), Equals, 1)

	lor, err = s.bucket.ListObjects(Prefix(objectName + "22"))
	c.Assert(err, IsNil)
	c.Assert(len(lor.Objects), Equals, 0)

	// list with max keys
	lor, err = s.bucket.ListObjects(Prefix(objectName), MaxKeys(2))
	c.Assert(err, IsNil)
	c.Assert(len(lor.Objects), Equals, 2)

	// list with marker
	lor, err = s.bucket.ListObjects(Marker(objectName+"1"), MaxKeys(1))
	c.Assert(err, IsNil)
	c.Assert(len(lor.Objects), Equals, 1)

	err = s.bucket.DeleteObject(objectName + "1")
	c.Assert(err, IsNil)
	err = s.bucket.DeleteObject(objectName + "2")
	c.Assert(err, IsNil)
	err = s.bucket.DeleteObject(objectName + "3")
	c.Assert(err, IsNil)
}

// TestListObjects
func (s *OssBucketSuite) TestListObjectsEncodingType(c *C) {
	objectName := objectNamePrefix + "床前明月光，疑是地上霜。举头望明月，低头思故乡。" + "tloet"

	for i := 0; i < 10; i++ {
		err := s.bucket.PutObject(objectName+strconv.Itoa(i), strings.NewReader(""))
		c.Assert(err, IsNil)
	}

	lor, err := s.bucket.ListObjects(Prefix(objectNamePrefix + "床前明月光，"))
	c.Assert(err, IsNil)
	c.Assert(len(lor.Objects), Equals, 10)

	lor, err = s.bucket.ListObjects(Prefix(objectNamePrefix + "床前明月光，"))
	c.Assert(err, IsNil)
	c.Assert(len(lor.Objects), Equals, 10)

	lor, err = s.bucket.ListObjects(Marker(objectNamePrefix + "床前明月光，疑是地上霜。举头望明月，低头思故乡。"))
	c.Assert(err, IsNil)
	c.Assert(len(lor.Objects), Equals, 10)

	lor, err = s.bucket.ListObjects(Prefix(objectNamePrefix + "床前明月光"))
	c.Assert(err, IsNil)
	for i, obj := range lor.Objects {
		c.Assert(obj.Key, Equals, objectNamePrefix+"床前明月光，疑是地上霜。举头望明月，低头思故乡。tloet"+strconv.Itoa(i))
	}

	for i := 0; i < 10; i++ {
		err = s.bucket.DeleteObject(objectName + strconv.Itoa(i))
		c.Assert(err, IsNil)
	}

	// 特殊字符
	objectName = "go go ` ~ ! @ # $ % ^ & * () - _ + =[] {} \\ | < > , . ? / 0"
	err = s.bucket.PutObject(objectName, strings.NewReader("明月几时有，把酒问青天"))
	c.Assert(err, IsNil)

	lor, err = s.bucket.ListObjects(Prefix(objectName))
	c.Assert(err, IsNil)
	c.Assert(len(lor.Objects), Equals, 1)

	err = s.bucket.DeleteObject(objectName)
	c.Assert(err, IsNil)

	objectName = "go/中国  日本  +-#&=*"
	err = s.bucket.PutObject(objectName, strings.NewReader("明月几时有，把酒问青天"))
	c.Assert(err, IsNil)

	lor, err = s.bucket.ListObjects(Prefix(objectName))
	c.Assert(err, IsNil)
	c.Assert(len(lor.Objects), Equals, 1)

	err = s.bucket.DeleteObject(objectName)
	c.Assert(err, IsNil)
}

// TestIsBucketExist
func (s *OssBucketSuite) TestIsObjectExist(c *C) {
	objectName := objectNamePrefix + "tibe"

	// Put three object
	err := s.bucket.PutObject(objectName+"1", strings.NewReader(""))
	c.Assert(err, IsNil)
	err = s.bucket.PutObject(objectName+"11", strings.NewReader(""))
	c.Assert(err, IsNil)
	err = s.bucket.PutObject(objectName+"111", strings.NewReader(""))
	c.Assert(err, IsNil)

	// exist
	exist, err := s.bucket.IsObjectExist(objectName + "11")
	c.Assert(err, IsNil)
	c.Assert(exist, Equals, true)

	exist, err = s.bucket.IsObjectExist(objectName + "1")
	c.Assert(err, IsNil)
	c.Assert(exist, Equals, true)

	exist, err = s.bucket.IsObjectExist(objectName + "111")
	c.Assert(err, IsNil)
	c.Assert(exist, Equals, true)

	// not exist
	exist, err = s.bucket.IsObjectExist(objectName + "1111")
	c.Assert(err, IsNil)
	c.Assert(exist, Equals, false)

	exist, err = s.bucket.IsObjectExist(objectName)
	c.Assert(err, IsNil)
	c.Assert(exist, Equals, false)

	err = s.bucket.DeleteObject(objectName + "1")
	c.Assert(err, IsNil)
	err = s.bucket.DeleteObject(objectName + "11")
	c.Assert(err, IsNil)
	err = s.bucket.DeleteObject(objectName + "111")
	c.Assert(err, IsNil)
}

// TestDeleteObject
func (s *OssBucketSuite) TestDeleteObject(c *C) {
	objectName := objectNamePrefix + "tdo"

	err := s.bucket.PutObject(objectName, strings.NewReader(""))
	c.Assert(err, IsNil)

	lor, err := s.bucket.ListObjects(Prefix(objectName))
	c.Assert(err, IsNil)
	c.Assert(len(lor.Objects), Equals, 1)

	// delete
	err = s.bucket.DeleteObject(objectName)
	c.Assert(err, IsNil)

	// duplicate delete
	err = s.bucket.DeleteObject(objectName)
	c.Assert(err, IsNil)

	lor, err = s.bucket.ListObjects(Prefix(objectName))
	c.Assert(err, IsNil)
	c.Assert(len(lor.Objects), Equals, 0)
}

// TestDeleteObjects
func (s *OssBucketSuite) TestDeleteObjects(c *C) {
	objectName := objectNamePrefix + "tdos"

	// delete object
	err := s.bucket.PutObject(objectName, strings.NewReader(""))
	c.Assert(err, IsNil)

	res, err := s.bucket.DeleteObjects([]string{objectName})
	c.Assert(err, IsNil)
	c.Assert(len(res.DeletedObjects), Equals, 1)

	lor, err := s.bucket.ListObjects(Prefix(objectName))
	c.Assert(err, IsNil)
	c.Assert(len(lor.Objects), Equals, 0)

	// delete objects
	err = s.bucket.PutObject(objectName+"1", strings.NewReader(""))
	c.Assert(err, IsNil)

	err = s.bucket.PutObject(objectName+"2", strings.NewReader(""))
	c.Assert(err, IsNil)

	res, err = s.bucket.DeleteObjects([]string{objectName + "1", objectName + "2"})
	c.Assert(err, IsNil)
	c.Assert(len(res.DeletedObjects), Equals, 2)

	lor, err = s.bucket.ListObjects(Prefix(objectName))
	c.Assert(err, IsNil)
	c.Assert(len(lor.Objects), Equals, 0)

	// delete 0
	_, err = s.bucket.DeleteObjects([]string{})
	c.Assert(err, NotNil)

	// DeleteObjectsQuiet
	err = s.bucket.PutObject(objectName+"1", strings.NewReader(""))
	c.Assert(err, IsNil)

	err = s.bucket.PutObject(objectName+"2", strings.NewReader(""))
	c.Assert(err, IsNil)

	res, err = s.bucket.DeleteObjects([]string{objectName + "1", objectName + "2"},
		DeleteObjectsQuiet(false))
	c.Assert(err, IsNil)
	c.Assert(len(res.DeletedObjects), Equals, 2)

	lor, err = s.bucket.ListObjects(Prefix(objectName))
	c.Assert(err, IsNil)
	c.Assert(len(lor.Objects), Equals, 0)

	// DeleteObjectsQuiet
	err = s.bucket.PutObject(objectName+"1", strings.NewReader(""))
	c.Assert(err, IsNil)

	err = s.bucket.PutObject(objectName+"2", strings.NewReader(""))
	c.Assert(err, IsNil)

	res, err = s.bucket.DeleteObjects([]string{objectName + "1", objectName + "2"},
		DeleteObjectsQuiet(true))
	c.Assert(err, IsNil)
	c.Assert(len(res.DeletedObjects), Equals, 0)

	lor, err = s.bucket.ListObjects(Prefix(objectName))
	c.Assert(err, IsNil)
	c.Assert(len(lor.Objects), Equals, 0)

	// EncodingType
	err = s.bucket.PutObject("中国人", strings.NewReader(""))
	c.Assert(err, IsNil)

	res, err = s.bucket.DeleteObjects([]string{"中国人"})
	c.Assert(err, IsNil)
	c.Assert(len(res.DeletedObjects), Equals, 1)
	c.Assert(res.DeletedObjects[0], Equals, "中国人")

	// EncodingType
	err = s.bucket.PutObject("中国人", strings.NewReader(""))
	c.Assert(err, IsNil)

	res, err = s.bucket.DeleteObjects([]string{"中国人"}, DeleteObjectsQuiet(false))
	c.Assert(err, IsNil)
	c.Assert(len(res.DeletedObjects), Equals, 1)
	c.Assert(res.DeletedObjects[0], Equals, "中国人")

	// EncodingType
	err = s.bucket.PutObject("中国人", strings.NewReader(""))
	c.Assert(err, IsNil)

	res, err = s.bucket.DeleteObjects([]string{"中国人"}, DeleteObjectsQuiet(true))
	c.Assert(err, IsNil)
	c.Assert(len(res.DeletedObjects), Equals, 0)

	// 特殊字符
	key := "A ' < > \" & ~ ` ! @ # $ % ^ & * ( ) [] {} - _ + = / | \\ ? . , : ; A"
	err = s.bucket.PutObject(key, strings.NewReader("value"))
	c.Assert(err, IsNil)

	_, err = s.bucket.DeleteObjects([]string{key})
	c.Assert(err, IsNil)

	ress, err := s.bucket.ListObjects(Prefix(key))
	c.Assert(err, IsNil)
	c.Assert(len(ress.Objects), Equals, 0)

	// not exist
	_, err = s.bucket.DeleteObjects([]string{"NotExistObject"})
	c.Assert(err, IsNil)
}

// TestSetObjectMeta
func (s *OssBucketSuite) TestSetObjectMeta(c *C) {
	objectName := objectNamePrefix + "tsom"

	err := s.bucket.PutObject(objectName, strings.NewReader(""))
	c.Assert(err, IsNil)

	err = s.bucket.SetObjectMeta(objectName,
		Expires(futureDate),
		Meta("myprop", "mypropval"))
	c.Assert(err, IsNil)

	meta, err := s.bucket.GetObjectDetailedMeta(objectName)
	c.Assert(err, IsNil)
	testLogger.Println("Meta:", meta)
	c.Assert(meta.Get("Expires"), Equals, futureDate.Format(http.TimeFormat))
	c.Assert(meta.Get("X-Oss-Meta-Myprop"), Equals, "mypropval")

	acl, err := s.bucket.GetObjectACL(objectName)
	c.Assert(err, IsNil)
	c.Assert(acl.ACL, Equals, "default")

	// invalid option
	err = s.bucket.SetObjectMeta(objectName, AcceptEncoding("url"))
	c.Assert(err, IsNil)

	// invalid option value
	err = s.bucket.SetObjectMeta(objectName, ServerSideEncryption("invalid"))
	c.Assert(err, NotNil)

	err = s.bucket.DeleteObject(objectName)
	c.Assert(err, IsNil)

	// no exist
	err = s.bucket.SetObjectMeta(objectName, Expires(futureDate))
	c.Assert(err, NotNil)
}

// TestGetObjectMeta
func (s *OssBucketSuite) TestGetObjectMeta(c *C) {
	objectName := objectNamePrefix + "tgom"

	// Put
	err := s.bucket.PutObject(objectName, strings.NewReader(""))
	c.Assert(err, IsNil)

	meta, err := s.bucket.GetObjectMeta(objectName)
	c.Assert(err, IsNil)
	c.Assert(len(meta) > 0, Equals, true)

	err = s.bucket.DeleteObject(objectName)
	c.Assert(err, IsNil)

	_, err = s.bucket.GetObjectMeta("NotExistObject")
	c.Assert(err, NotNil)
}

// TestGetObjectDetailedMeta
func (s *OssBucketSuite) TestGetObjectDetailedMeta(c *C) {
	objectName := objectNamePrefix + "tgodm"

	// Put
	err := s.bucket.PutObject(objectName, strings.NewReader(""),
		Expires(futureDate), Meta("myprop", "mypropval"))
	c.Assert(err, IsNil)

	// Check
	meta, err := s.bucket.GetObjectDetailedMeta(objectName)
	c.Assert(err, IsNil)
	testLogger.Println("GetObjectDetailedMeta:", meta)
	c.Assert(meta.Get("Expires"), Equals, futureDate.Format(http.TimeFormat))
	c.Assert(meta.Get("X-Oss-Meta-Myprop"), Equals, "mypropval")
	c.Assert(meta.Get("Content-Length"), Equals, "0")
	c.Assert(len(meta.Get("Date")) > 0, Equals, true)
	c.Assert(len(meta.Get("X-Oss-Request-Id")) > 0, Equals, true)
	c.Assert(len(meta.Get("Last-Modified")) > 0, Equals, true)

	// IfModifiedSince/IfModifiedSince
	_, err = s.bucket.GetObjectDetailedMeta(objectName, IfModifiedSince(futureDate))
	c.Assert(err, NotNil)

	meta, err = s.bucket.GetObjectDetailedMeta(objectName, IfUnmodifiedSince(futureDate))
	c.Assert(err, IsNil)
	c.Assert(meta.Get("Expires"), Equals, futureDate.Format(http.TimeFormat))
	c.Assert(meta.Get("X-Oss-Meta-Myprop"), Equals, "mypropval")

	// IfMatch/IfNoneMatch
	_, err = s.bucket.GetObjectDetailedMeta(objectName, IfNoneMatch(meta.Get("Etag")))
	c.Assert(err, NotNil)

	meta, err = s.bucket.GetObjectDetailedMeta(objectName, IfMatch(meta.Get("Etag")))
	c.Assert(err, IsNil)
	c.Assert(meta.Get("Expires"), Equals, futureDate.Format(http.TimeFormat))
	c.Assert(meta.Get("X-Oss-Meta-Myprop"), Equals, "mypropval")

	err = s.bucket.DeleteObject(objectName)
	c.Assert(err, IsNil)

	_, err = s.bucket.GetObjectDetailedMeta("NotExistObject")
	c.Assert(err, NotNil)
}

// TestSetAndGetObjectAcl
func (s *OssBucketSuite) TestSetAndGetObjectAcl(c *C) {
	objectName := objectNamePrefix + "tsgba"

	err := s.bucket.PutObject(objectName, strings.NewReader(""))
	c.Assert(err, IsNil)

	// default
	acl, err := s.bucket.GetObjectACL(objectName)
	c.Assert(err, IsNil)
	c.Assert(acl.ACL, Equals, "default")

	// set ACL_PUBLIC_RW
	err = s.bucket.SetObjectACL(objectName, ACLPublicReadWrite)
	c.Assert(err, IsNil)

	acl, err = s.bucket.GetObjectACL(objectName)
	c.Assert(err, IsNil)
	c.Assert(acl.ACL, Equals, string(ACLPublicReadWrite))

	// set ACL_PRIVATE
	err = s.bucket.SetObjectACL(objectName, ACLPrivate)
	c.Assert(err, IsNil)

	acl, err = s.bucket.GetObjectACL(objectName)
	c.Assert(err, IsNil)
	c.Assert(acl.ACL, Equals, string(ACLPrivate))

	// set ACL_PUBLIC_R
	err = s.bucket.SetObjectACL(objectName, ACLPublicRead)
	c.Assert(err, IsNil)

	acl, err = s.bucket.GetObjectACL(objectName)
	c.Assert(err, IsNil)
	c.Assert(acl.ACL, Equals, string(ACLPublicRead))

	err = s.bucket.DeleteObject(objectName)
	c.Assert(err, IsNil)
}

// TestSetAndGetObjectAclNegative
func (s *OssBucketSuite) TestSetAndGetObjectAclNegative(c *C) {
	objectName := objectNamePrefix + "tsgban"

	// object not exist
	err := s.bucket.SetObjectACL(objectName, ACLPublicRead)
	c.Assert(err, NotNil)
}

// TestCopyObject
func (s *OssBucketSuite) TestCopyObject(c *C) {
	objectName := objectNamePrefix + "tco"
	objectValue := "男儿何不带吴钩，收取关山五十州。请君暂上凌烟阁，若个书生万户侯？"

	err := s.bucket.PutObject(objectName, strings.NewReader(objectValue),
		ACL(ACLPublicRead), Meta("my", "myprop"))
	c.Assert(err, IsNil)

	// copy
	var objectNameDest = objectName + "dest"
	_, err = s.bucket.CopyObject(objectName, objectNameDest)
	c.Assert(err, IsNil)

	// check
	lor, err := s.bucket.ListObjects(Prefix(objectName))
	c.Assert(err, IsNil)
	testLogger.Println("objects:", lor.Objects)
	c.Assert(len(lor.Objects), Equals, 2)

	body, err := s.bucket.GetObject(objectName)
	c.Assert(err, IsNil)
	str, err := readBody(body)
	c.Assert(err, IsNil)
	c.Assert(str, Equals, objectValue)

	err = s.bucket.DeleteObject(objectNameDest)
	c.Assert(err, IsNil)

	// copy with constraints x-oss-copy-source-if-modified-since
	_, err = s.bucket.CopyObject(objectName, objectNameDest, CopySourceIfModifiedSince(futureDate))
	c.Assert(err, NotNil)
	testLogger.Println("CopyObject:", err)

	// copy with constraints x-oss-copy-source-if-unmodified-since
	_, err = s.bucket.CopyObject(objectName, objectNameDest, CopySourceIfUnmodifiedSince(futureDate))
	c.Assert(err, IsNil)

	// check
	lor, err = s.bucket.ListObjects(Prefix(objectName))
	c.Assert(err, IsNil)
	testLogger.Println("objects:", lor.Objects)
	c.Assert(len(lor.Objects), Equals, 2)

	body, err = s.bucket.GetObject(objectName)
	c.Assert(err, IsNil)
	str, err = readBody(body)
	c.Assert(err, IsNil)
	c.Assert(str, Equals, objectValue)

	err = s.bucket.DeleteObject(objectNameDest)
	c.Assert(err, IsNil)

	// copy with constraints x-oss-copy-source-if-match
	meta, err := s.bucket.GetObjectDetailedMeta(objectName)
	c.Assert(err, IsNil)
	testLogger.Println("GetObjectDetailedMeta:", meta)

	_, err = s.bucket.CopyObject(objectName, objectNameDest, CopySourceIfMatch(meta.Get("Etag")))
	c.Assert(err, IsNil)

	// check
	body, err = s.bucket.GetObject(objectName)
	c.Assert(err, IsNil)
	str, err = readBody(body)
	c.Assert(err, IsNil)
	c.Assert(str, Equals, objectValue)

	err = s.bucket.DeleteObject(objectNameDest)
	c.Assert(err, IsNil)

	// copy with constraints x-oss-copy-source-if-none-match
	_, err = s.bucket.CopyObject(objectName, objectNameDest, CopySourceIfNoneMatch(meta.Get("Etag")))
	c.Assert(err, NotNil)

	// copy with constraints x-oss-metadata-directive
	_, err = s.bucket.CopyObject(objectName, objectNameDest, Meta("my", "mydestprop"),
		MetadataDirective(MetaCopy))
	c.Assert(err, IsNil)

	// check
	body, err = s.bucket.GetObject(objectName)
	c.Assert(err, IsNil)
	str, err = readBody(body)
	c.Assert(err, IsNil)
	c.Assert(str, Equals, objectValue)

	destMeta, err := s.bucket.GetObjectDetailedMeta(objectNameDest)
	c.Assert(err, IsNil)
	c.Assert(meta.Get("X-Oss-Meta-My"), Equals, "myprop")

	acl, err := s.bucket.GetObjectACL(objectNameDest)
	c.Assert(err, IsNil)
	c.Assert(acl.ACL, Equals, "default")

	err = s.bucket.DeleteObject(objectNameDest)
	c.Assert(err, IsNil)

	// copy with constraints x-oss-metadata-directive and self defined dest object meta
	options := []Option{
		ObjectACL(ACLPublicReadWrite),
		Meta("my", "mydestprop"),
		MetadataDirective(MetaReplace),
	}
	_, err = s.bucket.CopyObject(objectName, objectNameDest, options...)
	c.Assert(err, IsNil)

	// check
	body, err = s.bucket.GetObject(objectName)
	c.Assert(err, IsNil)
	str, err = readBody(body)
	c.Assert(err, IsNil)
	c.Assert(str, Equals, objectValue)

	destMeta, err = s.bucket.GetObjectDetailedMeta(objectNameDest)
	c.Assert(err, IsNil)
	c.Assert(destMeta.Get("X-Oss-Meta-My"), Equals, "mydestprop")

	acl, err = s.bucket.GetObjectACL(objectNameDest)
	c.Assert(err, IsNil)
	c.Assert(acl.ACL, Equals, string(ACLPublicReadWrite))

	err = s.bucket.DeleteObject(objectNameDest)
	c.Assert(err, IsNil)

	err = s.bucket.DeleteObject(objectName)
	c.Assert(err, IsNil)
}

// TestCopyObjectToOrFrom
func (s *OssBucketSuite) TestCopyObjectToOrFrom(c *C) {
	objectName := objectNamePrefix + "tcotof"
	objectValue := "男儿何不带吴钩，收取关山五十州。请君暂上凌烟阁，若个书生万户侯？"
	destBucket := bucketName + "-dest"
	objectNameDest := objectName + "dest"

	s.client.CreateBucket(destBucket)

	destBuck, err := s.client.Bucket(destBucket)
	c.Assert(err, IsNil)

	err = s.bucket.PutObject(objectName, strings.NewReader(objectValue))
	c.Assert(err, IsNil)

	// copy from
	_, err = destBuck.CopyObjectFrom(bucketName, objectName, objectNameDest)
	c.Assert(err, IsNil)

	// check
	body, err := destBuck.GetObject(objectNameDest)
	c.Assert(err, IsNil)
	str, err := readBody(body)
	c.Assert(err, IsNil)
	c.Assert(str, Equals, objectValue)

	err = s.bucket.DeleteObject(objectName)
	c.Assert(err, IsNil)

	// copy to
	_, err = destBuck.CopyObjectTo(bucketName, objectName, objectNameDest)
	c.Assert(err, IsNil)

	// check
	body, err = s.bucket.GetObject(objectName)
	c.Assert(err, IsNil)
	str, err = readBody(body)
	c.Assert(err, IsNil)
	c.Assert(str, Equals, objectValue)

	// clean
	err = destBuck.DeleteObject(objectNameDest)
	c.Assert(err, IsNil)

	err = s.bucket.DeleteObject(objectName)
	c.Assert(err, IsNil)

	err = s.client.DeleteBucket(destBucket)
	c.Assert(err, IsNil)
}

// TestCopyObjectToOrFromNegative
func (s *OssBucketSuite) TestCopyObjectToOrFromNegative(c *C) {
	objectName := objectNamePrefix + "tcotofn"
	destBucket := bucketName + "-destn"
	objectNameDest := objectName + "destn"

	// object no exist
	_, err := s.bucket.CopyObjectTo(bucketName, objectName, objectNameDest)
	c.Assert(err, NotNil)

	// bucket no exist
	_, err = s.bucket.CopyObjectFrom(destBucket, objectNameDest, objectName)
	c.Assert(err, NotNil)
}

// TestAppendObject
func (s *OssBucketSuite) TestAppendObject(c *C) {
	objectName := objectNamePrefix + "tao"
	objectValue := "昨夜雨疏风骤，浓睡不消残酒。试问卷帘人，却道海棠依旧。知否？知否？应是绿肥红瘦。"
	var val = []byte(objectValue)
	var localFile = "testx.txt"
	var nextPos int64
	var midPos = 1 + rand.Intn(len(val)-1)

	var err = createFileAndWrite(localFile+"1", val[0:midPos])
	c.Assert(err, IsNil)
	err = createFileAndWrite(localFile+"2", val[midPos:])
	c.Assert(err, IsNil)

	// string append
	nextPos, err = s.bucket.AppendObject(objectName, strings.NewReader("昨夜雨疏风骤，浓睡不消残酒。试问卷帘人，"), nextPos)
	c.Assert(err, IsNil)
	nextPos, err = s.bucket.AppendObject(objectName, strings.NewReader("却道海棠依旧。知否？知否？应是绿肥红瘦。"), nextPos)
	c.Assert(err, IsNil)

	body, err := s.bucket.GetObject(objectName)
	c.Assert(err, IsNil)
	str, err := readBody(body)
	c.Assert(err, IsNil)
	c.Assert(str, Equals, objectValue)

	err = s.bucket.DeleteObject(objectName)
	c.Assert(err, IsNil)

	// byte append
	nextPos = 0
	nextPos, err = s.bucket.AppendObject(objectName, bytes.NewReader(val[0:midPos]), nextPos)
	c.Assert(err, IsNil)
	nextPos, err = s.bucket.AppendObject(objectName, bytes.NewReader(val[midPos:]), nextPos)
	c.Assert(err, IsNil)

	body, err = s.bucket.GetObject(objectName)
	c.Assert(err, IsNil)
	str, err = readBody(body)
	c.Assert(err, IsNil)
	c.Assert(str, Equals, objectValue)

	err = s.bucket.DeleteObject(objectName)
	c.Assert(err, IsNil)

	// file append
	options := []Option{
		ObjectACL(ACLPublicReadWrite),
		Meta("my", "myprop"),
	}

	fd, err := os.Open(localFile + "1")
	c.Assert(err, IsNil)
	defer fd.Close()
	nextPos = 0
	nextPos, err = s.bucket.AppendObject(objectName, fd, nextPos, options...)
	c.Assert(err, IsNil)

	meta, err := s.bucket.GetObjectDetailedMeta(objectName)
	c.Assert(err, IsNil)
	testLogger.Println("GetObjectDetailedMeta:", meta, ",", nextPos)
	c.Assert(meta.Get("X-Oss-Object-Type"), Equals, "Appendable")
	c.Assert(meta.Get("X-Oss-Meta-My"), Equals, "myprop")
	c.Assert(meta.Get("x-oss-Meta-Mine"), Equals, "")
	c.Assert(meta.Get("X-Oss-Next-Append-Position"), Equals, strconv.FormatInt(nextPos, 10))

	acl, err := s.bucket.GetObjectACL(objectName)
	c.Assert(err, IsNil)
	testLogger.Println("GetObjectACL:", acl)
	c.Assert(acl.ACL, Equals, string(ACLPublicReadWrite))

	// second append
	options = []Option{
		ObjectACL(ACLPublicRead),
		Meta("my", "myproptwo"),
		Meta("mine", "mypropmine"),
	}
	fd, err = os.Open(localFile + "2")
	c.Assert(err, IsNil)
	defer fd.Close()
	nextPos, err = s.bucket.AppendObject(objectName, fd, nextPos, options...)
	c.Assert(err, IsNil)

	body, err = s.bucket.GetObject(objectName)
	c.Assert(err, IsNil)
	str, err = readBody(body)
	c.Assert(err, IsNil)
	c.Assert(str, Equals, objectValue)

	meta, err = s.bucket.GetObjectDetailedMeta(objectName)
	c.Assert(err, IsNil)
	testLogger.Println("GetObjectDetailedMeta xxx:", meta)
	c.Assert(meta.Get("X-Oss-Object-Type"), Equals, "Appendable")
	c.Assert(meta.Get("X-Oss-Meta-My"), Equals, "myprop")
	c.Assert(meta.Get("x-Oss-Meta-Mine"), Equals, "")
	c.Assert(meta.Get("X-Oss-Next-Append-Position"), Equals, strconv.FormatInt(nextPos, 10))

	acl, err = s.bucket.GetObjectACL(objectName)
	c.Assert(err, IsNil)
	c.Assert(acl.ACL, Equals, string(ACLPublicRead))

	err = s.bucket.DeleteObject(objectName)
	c.Assert(err, IsNil)
}

// TestAppendObjectNegative
func (s *OssBucketSuite) TestAppendObjectNegative(c *C) {
	objectName := objectNamePrefix + "taon"
	nextPos := int64(0)

	nextPos, err := s.bucket.AppendObject(objectName, strings.NewReader("ObjectValue"), nextPos)
	c.Assert(err, IsNil)

	nextPos, err = s.bucket.AppendObject(objectName, strings.NewReader("ObjectValue"), 0)
	c.Assert(err, NotNil)

	err = s.bucket.DeleteObject(objectName)
	c.Assert(err, IsNil)
}

// TestContentType
func (s *OssBucketSuite) TestAddContentType(c *C) {
	opts := addContentType(nil, "abc.txt")
	typ, err := findOption(opts, HTTPHeaderContentType, "")
	c.Assert(err, IsNil)
	c.Assert(typ, Equals, "text/plain; charset=utf-8")

	opts = addContentType(nil)
	typ, err = findOption(opts, HTTPHeaderContentType, "")
	c.Assert(err, IsNil)
	c.Assert(len(opts), Equals, 1)
	c.Assert(typ, Equals, "application/octet-stream")

	opts = addContentType(nil, "abc.txt", "abc.pdf")
	typ, err = findOption(opts, HTTPHeaderContentType, "")
	c.Assert(err, IsNil)
	c.Assert(typ, Equals, "text/plain; charset=utf-8")

	opts = addContentType(nil, "abc", "abc.txt", "abc.pdf")
	typ, err = findOption(opts, HTTPHeaderContentType, "")
	c.Assert(err, IsNil)
	c.Assert(typ, Equals, "text/plain; charset=utf-8")

	opts = addContentType(nil, "abc", "abc", "edf")
	typ, err = findOption(opts, HTTPHeaderContentType, "")
	c.Assert(err, IsNil)
	c.Assert(typ, Equals, "application/octet-stream")

	opts = addContentType([]Option{Meta("meta", "my")}, "abc", "abc.txt", "abc.pdf")
	typ, err = findOption(opts, HTTPHeaderContentType, "")
	c.Assert(err, IsNil)
	c.Assert(len(opts), Equals, 2)
	c.Assert(typ, Equals, "text/plain; charset=utf-8")
}

func (s *OssBucketSuite) TestGetConfig(c *C) {
	client, err := New(endpoint, accessID, accessKey, UseCname(true),
		Timeout(11, 12), SecurityToken("token"), EnableMD5(false))
	c.Assert(err, IsNil)

	bucket, err := client.Bucket(bucketName)
	c.Assert(err, IsNil)

	c.Assert(bucket.getConfig().HTTPTimeout.ConnectTimeout, Equals, time.Second*11)
	c.Assert(bucket.getConfig().HTTPTimeout.ReadWriteTimeout, Equals, time.Second*12)
	c.Assert(bucket.getConfig().HTTPTimeout.HeaderTimeout, Equals, time.Second*12)
	c.Assert(bucket.getConfig().HTTPTimeout.LongTimeout, Equals, time.Second*12*10)

	c.Assert(bucket.getConfig().SecurityToken, Equals, "token")
	c.Assert(bucket.getConfig().IsCname, Equals, true)
	c.Assert(bucket.getConfig().IsEnableMD5, Equals, false)
}

func (s *OssBucketSuite) TestSTSToken(c *C) {
	objectName := objectNamePrefix + "tst"
	objectValue := "红藕香残玉簟秋。轻解罗裳，独上兰舟。云中谁寄锦书来？雁字回时，月满西楼。"

	stsClient := sts.NewClient(stsaccessID, stsaccessKey, stsARN, "oss_test_sess")

	resp, err := stsClient.AssumeRole(1800)
	c.Assert(err, IsNil)

	client, err := New(endpoint, resp.Credentials.AccessKeyId, resp.Credentials.AccessKeySecret,
		SecurityToken(resp.Credentials.SecurityToken))
	c.Assert(err, IsNil)

	bucket, err := client.Bucket(bucketName)
	c.Assert(err, IsNil)

	// Put
	err = bucket.PutObject(objectName, strings.NewReader(objectValue))
	c.Assert(err, IsNil)

	// Get
	body, err := s.bucket.GetObject(objectName)
	c.Assert(err, IsNil)
	str, err := readBody(body)
	c.Assert(err, IsNil)
	c.Assert(str, Equals, objectValue)

	// List
	lor, err := bucket.ListObjects()
	c.Assert(err, IsNil)
	testLogger.Println("Objects:", lor.Objects)

	// Delete
	err = bucket.DeleteObject(objectName)
	c.Assert(err, IsNil)
}

func (s *OssBucketSuite) TestSTSTonekNegative(c *C) {
	objectName := objectNamePrefix + "tstg"
	localFile := objectName + ".jpg"

	client, err := New(endpoint, accessID, accessKey, SecurityToken("Invalid"))
	c.Assert(err, IsNil)

	_, err = client.ListBuckets()
	c.Assert(err, NotNil)

	bucket, err := client.Bucket(bucketName)
	c.Assert(err, IsNil)

	err = bucket.PutObject(objectName, strings.NewReader(""))
	c.Assert(err, NotNil)

	err = bucket.PutObjectFromFile(objectName, "")
	c.Assert(err, NotNil)

	_, err = bucket.GetObject(objectName)
	c.Assert(err, NotNil)

	err = bucket.GetObjectToFile(objectName, "")
	c.Assert(err, NotNil)

	_, err = bucket.ListObjects()
	c.Assert(err, NotNil)

	err = bucket.SetObjectACL(objectName, ACLPublicRead)
	c.Assert(err, NotNil)

	_, err = bucket.GetObjectACL(objectName)
	c.Assert(err, NotNil)

	err = bucket.UploadFile(objectName, localFile, MinPartSize)
	c.Assert(err, NotNil)

	err = bucket.DownloadFile(objectName, localFile, MinPartSize)
	c.Assert(err, NotNil)

	_, err = bucket.IsObjectExist(objectName)
	c.Assert(err, NotNil)

	_, err = bucket.ListMultipartUploads()
	c.Assert(err, NotNil)

	err = bucket.DeleteObject(objectName)
	c.Assert(err, NotNil)

	_, err = bucket.DeleteObjects([]string{objectName})
	c.Assert(err, NotNil)

	err = client.DeleteBucket(bucketName)
	c.Assert(err, NotNil)
}

func (s *OssBucketSuite) TestUploadBigFile(c *C) {
	objectName := objectNamePrefix + "tubf"
	bigFile := "D:\\tmp\\bigfile.zip"
	newFile := "D:\\tmp\\newbigfile.zip"

	exist, err := isFileExist(bigFile)
	c.Assert(err, IsNil)
	if !exist {
		return
	}

	// Put
	start := GetNowSec()
	err = s.bucket.PutObjectFromFile(objectName, bigFile)
	c.Assert(err, IsNil)
	end := GetNowSec()
	testLogger.Println("Put big file:", bigFile, "use sec:", end-start)

	// Check
	start = GetNowSec()
	err = s.bucket.GetObjectToFile(objectName, newFile)
	c.Assert(err, IsNil)
	end = GetNowSec()
	testLogger.Println("Get big file:", bigFile, "use sec:", end-start)

	start = GetNowSec()
	eq, err := compareFiles(bigFile, newFile)
	c.Assert(err, IsNil)
	c.Assert(eq, Equals, true)
	end = GetNowSec()
	testLogger.Println("Compare big file:", bigFile, "use sec:", end-start)

	err = s.bucket.DeleteObject(objectName)
	c.Assert(err, IsNil)
}

func (s *OssBucketSuite) TestSymlink(c *C) {
	objectName := objectNamePrefix + "符号链接"
	targetObjectName := objectNamePrefix + "符号链接目标文件"

	err := s.bucket.DeleteObject(objectName)
	c.Assert(err, IsNil)

	err = s.bucket.DeleteObject(targetObjectName)
	c.Assert(err, IsNil)

	meta, err := s.bucket.GetSymlink(objectName)
	c.Assert(err, NotNil)

	// Put symlink
	err = s.bucket.PutSymlink(objectName, targetObjectName)
	c.Assert(err, IsNil)

	err = s.bucket.PutObject(targetObjectName, strings.NewReader("target"))
	c.Assert(err, IsNil)

	err = s.bucket.PutSymlink(objectName, targetObjectName)
	c.Assert(err, IsNil)

	meta, err = s.bucket.GetSymlink(objectName)
	c.Assert(err, IsNil)
	c.Assert(meta.Get(HTTPHeaderOssSymlinkTarget), Equals, targetObjectName)

	// List object
	lor, err := s.bucket.ListObjects()
	c.Assert(err, IsNil)
	exist, v := s.getObject(lor.Objects, objectName)
	c.Assert(exist, Equals, true)
	c.Assert(v.Type, Equals, "Symlink")

	body, err := s.bucket.GetObject(objectName)
	c.Assert(err, IsNil)
	str, err := readBody(body)
	c.Assert(err, IsNil)
	c.Assert(str, Equals, "target")

	meta, err = s.bucket.GetSymlink(targetObjectName)
	c.Assert(err, NotNil)

	err = s.bucket.PutObject(objectName, strings.NewReader("src"))
	c.Assert(err, IsNil)

	body, err = s.bucket.GetObject(objectName)
	c.Assert(err, IsNil)
	str, err = readBody(body)
	c.Assert(err, IsNil)
	c.Assert(str, Equals, "src")

	err = s.bucket.DeleteObject(objectName)
	c.Assert(err, IsNil)

	err = s.bucket.DeleteObject(targetObjectName)
	c.Assert(err, IsNil)

	// put symlink again
	objectName = objectNamePrefix + "symlink"
	targetObjectName = objectNamePrefix + "symlink-target"

	err = s.bucket.PutSymlink(objectName, targetObjectName)
	c.Assert(err, IsNil)

	err = s.bucket.PutObject(targetObjectName, strings.NewReader("target1"))
	c.Assert(err, IsNil)

	meta, err = s.bucket.GetSymlink(objectName)
	c.Assert(err, IsNil)
	c.Assert(meta.Get(HTTPHeaderOssSymlinkTarget), Equals, targetObjectName)

	body, err = s.bucket.GetObject(objectName)
	c.Assert(err, IsNil)
	str, err = readBody(body)
	c.Assert(err, IsNil)
	c.Assert(str, Equals, "target1")

	err = s.bucket.DeleteObject(objectName)
	c.Assert(err, IsNil)

	err = s.bucket.DeleteObject(targetObjectName)
	c.Assert(err, IsNil)
}

// TestRestoreObject
func (s *OssBucketSuite) TestRestoreObject(c *C) {
	objectName := objectNamePrefix + "restore"

	// List Object
	lor, err := s.archiveBucket.ListObjects()
	c.Assert(err, IsNil)
	left := len(lor.Objects)

	// Put three object
	err = s.archiveBucket.PutObject(objectName, strings.NewReader(""))
	c.Assert(err, IsNil)

	// List
	lor, err = s.archiveBucket.ListObjects()
	c.Assert(err, IsNil)
	c.Assert(len(lor.Objects), Equals, left+1)
	for _, object := range lor.Objects {
		c.Assert(object.StorageClass, Equals, string(StorageArchive))
		c.Assert(object.Type, Equals, "Normal")
	}

	// Head Object
	meta, err := s.archiveBucket.GetObjectDetailedMeta(objectName)
	c.Assert(err, IsNil)
	_, ok := meta["X-Oss-Restore"]
	c.Assert(ok, Equals, false)
	c.Assert(meta.Get("X-Oss-Storage-Class"), Equals, "Archive")

	// Error Restore
	err = s.archiveBucket.RestoreObject("notexistobject")
	c.Assert(err, NotNil)

	// Restore Object
	err = s.archiveBucket.RestoreObject(objectName)
	c.Assert(err, IsNil)

	// Head Object
	meta, err = s.archiveBucket.GetObjectDetailedMeta(objectName)
	c.Assert(err, IsNil)
	c.Assert(meta.Get("X-Oss-Restore"), Equals, "ongoing-request=\"true\"")
	c.Assert(meta.Get("X-Oss-Storage-Class"), Equals, "Archive")
}

// private
func createFileAndWrite(fileName string, data []byte) error {
	os.Remove(fileName)

	fo, err := os.Create(fileName)
	if err != nil {
		return err
	}
	defer fo.Close()

	bytes, err := fo.Write(data)
	if err != nil {
		return err
	}

	if bytes != len(data) {
		return fmt.Errorf(fmt.Sprintf("write %d bytes not equal data length %d", bytes, len(data)))
	}

	return nil
}

// compare the content between fileL and fileR
func compareFiles(fileL string, fileR string) (bool, error) {
	finL, err := os.Open(fileL)
	if err != nil {
		return false, err
	}
	defer finL.Close()

	finR, err := os.Open(fileR)
	if err != nil {
		return false, err
	}
	defer finR.Close()

	statL, err := finL.Stat()
	if err != nil {
		return false, err
	}

	statR, err := finR.Stat()
	if err != nil {
		return false, err
	}

	if statL.Size() != statR.Size() {
		return false, nil
	}

	size := statL.Size()
	if size > 102400 {
		size = 102400
	}

	bufL := make([]byte, size)
	bufR := make([]byte, size)
	for {
		n, _ := finL.Read(bufL)
		if 0 == n {
			break
		}

		n, _ = finR.Read(bufR)
		if 0 == n {
			break
		}

		if !bytes.Equal(bufL, bufR) {
			return false, nil
		}
	}

	return true, nil
}

// compare the content of file and data
func compareFileData(file string, data []byte) (bool, error) {
	fin, err := os.Open(file)
	if err != nil {
		return false, err
	}
	defer fin.Close()

	stat, err := fin.Stat()
	if err != nil {
		return false, err
	}

	if stat.Size() != (int64)(len(data)) {
		return false, nil
	}

	buf := make([]byte, stat.Size())
	n, err := fin.Read(buf)
	if err != nil {
		return false, err
	}
	if stat.Size() != (int64)(n) {
		return false, errors.New("read error")
	}

	if !bytes.Equal(buf, data) {
		return false, nil
	}

	return true, nil
}

func walkDir(dirPth, suffix string) ([]string, error) {
	var files = []string{}
	suffix = strings.ToUpper(suffix)
	err := filepath.Walk(dirPth,
		func(filename string, fi os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if fi.IsDir() {
				return nil
			}
			if strings.HasSuffix(strings.ToUpper(fi.Name()), suffix) {
				files = append(files, filename)
			}
			return nil
		})
	return files, err
}

func removeTempFiles(path string, prefix string) error {
	files, err := walkDir(path, prefix)
	if err != nil {
		return nil
	}

	for _, file := range files {
		os.Remove(file)
	}

	return nil
}

func isFileExist(filename string) (bool, error) {
	_, err := os.Stat(filename)
	if err != nil && os.IsNotExist(err) {
		return false, nil
	} else if err != nil {
		return false, err
	} else {
		return true, nil
	}
}

func readBody(body io.ReadCloser) (string, error) {
	data, err := ioutil.ReadAll(body)
	body.Close()
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (s *OssBucketSuite) getObject(objects []ObjectProperties, object string) (bool, ObjectProperties) {
	for _, v := range objects {
		if v.Key == object {
			return true, v
		}
	}
	return false, ObjectProperties{}
}
