// multipart test

package oss

import (
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"time"

	. "gopkg.in/check.v1"
)

type OssBucketMultipartSuite struct {
	client *Client
	bucket *Bucket
}

var _ = Suite(&OssBucketMultipartSuite{})

// Run once when the suite starts running
func (s *OssBucketMultipartSuite) SetUpSuite(c *C) {
	client, err := New(endpoint, accessID, accessKey)
	c.Assert(err, IsNil)
	s.client = client

	s.client.CreateBucket(bucketName)
	time.Sleep(5 * time.Second)

	bucket, err := s.client.Bucket(bucketName)
	c.Assert(err, IsNil)
	s.bucket = bucket

	// Delete Part
	lmur, err := s.bucket.ListMultipartUploads()
	c.Assert(err, IsNil)

	for _, upload := range lmur.Uploads {
		var imur = InitiateMultipartUploadResult{Bucket: s.bucket.BucketName,
			Key: upload.Key, UploadID: upload.UploadID}
		err = s.bucket.AbortMultipartUpload(imur)
		c.Assert(err, IsNil)
	}

	// Delete Objects
	lor, err := s.bucket.ListObjects()
	c.Assert(err, IsNil)

	for _, object := range lor.Objects {
		err = s.bucket.DeleteObject(object.Key)
		c.Assert(err, IsNil)
	}

	testLogger.Println("test multipart started")
}

// Run before each test or benchmark starts running
func (s *OssBucketMultipartSuite) TearDownSuite(c *C) {
	// Delete Part
	lmur, err := s.bucket.ListMultipartUploads()
	c.Assert(err, IsNil)

	for _, upload := range lmur.Uploads {
		var imur = InitiateMultipartUploadResult{Bucket: s.bucket.BucketName,
			Key: upload.Key, UploadID: upload.UploadID}
		err = s.bucket.AbortMultipartUpload(imur)
		c.Assert(err, IsNil)
	}

	// Delete Objects
	lor, err := s.bucket.ListObjects()
	c.Assert(err, IsNil)

	for _, object := range lor.Objects {
		err = s.bucket.DeleteObject(object.Key)
		c.Assert(err, IsNil)
	}

	testLogger.Println("test multipart completed")
}

// Run after each test or benchmark runs
func (s *OssBucketMultipartSuite) SetUpTest(c *C) {
	err := removeTempFiles("../oss", ".jpg")
	c.Assert(err, IsNil)
}

// Run once after all tests or benchmarks have finished running
func (s *OssBucketMultipartSuite) TearDownTest(c *C) {
	err := removeTempFiles("../oss", ".jpg")
	c.Assert(err, IsNil)

	err = removeTempFiles("../oss", ".temp")
	c.Assert(err, IsNil)

	err = removeTempFiles("../oss", ".txt1")
	c.Assert(err, IsNil)

	err = removeTempFiles("../oss", ".txt2")
	c.Assert(err, IsNil)
}

// TestMultipartUpload
func (s *OssBucketMultipartSuite) TestMultipartUpload(c *C) {
	objectName := objectNamePrefix + "tmu"
	var fileName = "../sample/BingWallpaper-2015-11-07.jpg"

	chunks, err := SplitFileByPartNum(fileName, 3)
	c.Assert(err, IsNil)
	testLogger.Println("chunks:", chunks)

	options := []Option{
		Expires(futureDate), Meta("my", "myprop"),
	}

	fd, err := os.Open(fileName)
	c.Assert(err, IsNil)
	defer fd.Close()

	imur, err := s.bucket.InitiateMultipartUpload(objectName, options...)
	c.Assert(err, IsNil)
	var parts []UploadPart
	for _, chunk := range chunks {
		fd.Seek(chunk.Offset, os.SEEK_SET)
		part, err := s.bucket.UploadPart(imur, fd, chunk.Size, chunk.Number)
		c.Assert(err, IsNil)
		parts = append(parts, part)
	}

	cmur, err := s.bucket.CompleteMultipartUpload(imur, parts)
	c.Assert(err, IsNil)
	testLogger.Println("cmur:", cmur)

	meta, err := s.bucket.GetObjectDetailedMeta(objectName)
	c.Assert(err, IsNil)
	testLogger.Println("GetObjectDetailedMeta:", meta)
	c.Assert(meta.Get("X-Oss-Meta-My"), Equals, "myprop")
	c.Assert(meta.Get("Expires"), Equals, futureDate.Format(http.TimeFormat))
	c.Assert(meta.Get("X-Oss-Object-Type"), Equals, "Multipart")

	err = s.bucket.GetObjectToFile(objectName, "newpic1.jpg")
	c.Assert(err, IsNil)

	err = s.bucket.DeleteObject(objectName)
	c.Assert(err, IsNil)
}

// TestMultipartUpload
func (s *OssBucketMultipartSuite) TestMultipartUploadFromFile(c *C) {
	objectName := objectNamePrefix + "tmuff"
	var fileName = "../sample/BingWallpaper-2015-11-07.jpg"

	chunks, err := SplitFileByPartNum(fileName, 3)
	c.Assert(err, IsNil)
	testLogger.Println("chunks:", chunks)

	options := []Option{
		Expires(futureDate), Meta("my", "myprop"),
	}
	imur, err := s.bucket.InitiateMultipartUpload(objectName, options...)
	c.Assert(err, IsNil)
	var parts []UploadPart
	for _, chunk := range chunks {
		part, err := s.bucket.UploadPartFromFile(imur, fileName, chunk.Offset, chunk.Size, chunk.Number)
		c.Assert(err, IsNil)
		parts = append(parts, part)
	}

	cmur, err := s.bucket.CompleteMultipartUpload(imur, parts)
	c.Assert(err, IsNil)
	testLogger.Println("cmur:", cmur)

	meta, err := s.bucket.GetObjectDetailedMeta(objectName)
	c.Assert(err, IsNil)
	testLogger.Println("GetObjectDetailedMeta:", meta)
	c.Assert(meta.Get("X-Oss-Meta-My"), Equals, "myprop")
	c.Assert(meta.Get("Expires"), Equals, futureDate.Format(http.TimeFormat))
	c.Assert(meta.Get("X-Oss-Object-Type"), Equals, "Multipart")

	err = s.bucket.GetObjectToFile(objectName, "newpic1.jpg")
	c.Assert(err, IsNil)

	err = s.bucket.DeleteObject(objectName)
	c.Assert(err, IsNil)
}

// TestUploadPartCopy
func (s *OssBucketMultipartSuite) TestUploadPartCopy(c *C) {
	objectSrc := objectNamePrefix + "tupc" + "src"
	objectDesc := objectNamePrefix + "tupc" + "desc"
	var fileName = "../sample/BingWallpaper-2015-11-07.jpg"

	chunks, err := SplitFileByPartNum(fileName, 3)
	c.Assert(err, IsNil)
	testLogger.Println("chunks:", chunks)

	err = s.bucket.PutObjectFromFile(objectSrc, fileName)
	c.Assert(err, IsNil)

	options := []Option{
		Expires(futureDate), Meta("my", "myprop"),
	}
	imur, err := s.bucket.InitiateMultipartUpload(objectDesc, options...)
	c.Assert(err, IsNil)
	var parts []UploadPart
	for _, chunk := range chunks {
		part, err := s.bucket.UploadPartCopy(imur, bucketName, objectSrc, chunk.Offset, chunk.Size, (int)(chunk.Number))
		c.Assert(err, IsNil)
		parts = append(parts, part)
	}

	cmur, err := s.bucket.CompleteMultipartUpload(imur, parts)
	c.Assert(err, IsNil)
	testLogger.Println("cmur:", cmur)

	meta, err := s.bucket.GetObjectDetailedMeta(objectDesc)
	c.Assert(err, IsNil)
	testLogger.Println("GetObjectDetailedMeta:", meta)
	c.Assert(meta.Get("X-Oss-Meta-My"), Equals, "myprop")
	c.Assert(meta.Get("Expires"), Equals, futureDate.Format(http.TimeFormat))
	c.Assert(meta.Get("X-Oss-Object-Type"), Equals, "Multipart")

	err = s.bucket.GetObjectToFile(objectDesc, "newpic2.jpg")
	c.Assert(err, IsNil)

	err = s.bucket.DeleteObject(objectSrc)
	c.Assert(err, IsNil)
	err = s.bucket.DeleteObject(objectDesc)
	c.Assert(err, IsNil)
}

func (s *OssBucketMultipartSuite) TestListUploadedParts(c *C) {
	objectName := objectNamePrefix + "tlup"
	objectSrc := objectNamePrefix + "tlup" + "src"
	objectDesc := objectNamePrefix + "tlup" + "desc"
	var fileName = "../sample/BingWallpaper-2015-11-07.jpg"

	chunks, err := SplitFileByPartSize(fileName, 100*1024)
	c.Assert(err, IsNil)
	testLogger.Println("chunks:", chunks)

	err = s.bucket.PutObjectFromFile(objectSrc, fileName)
	c.Assert(err, IsNil)

	// upload
	imurUpload, err := s.bucket.InitiateMultipartUpload(objectName)
	var partsUpload []UploadPart
	for _, chunk := range chunks {
		part, err := s.bucket.UploadPartFromFile(imurUpload, fileName, chunk.Offset, chunk.Size, (int)(chunk.Number))
		c.Assert(err, IsNil)
		partsUpload = append(partsUpload, part)
	}

	// copy
	imurCopy, err := s.bucket.InitiateMultipartUpload(objectDesc)
	var partsCopy []UploadPart
	for _, chunk := range chunks {
		part, err := s.bucket.UploadPartCopy(imurCopy, bucketName, objectSrc, chunk.Offset, chunk.Size, (int)(chunk.Number))
		c.Assert(err, IsNil)
		partsCopy = append(partsCopy, part)
	}

	// list
	lupr, err := s.bucket.ListUploadedParts(imurUpload)
	c.Assert(err, IsNil)
	testLogger.Println("lupr:", lupr)
	c.Assert(len(lupr.UploadedParts), Equals, len(chunks))

	lupr, err = s.bucket.ListUploadedParts(imurCopy)
	c.Assert(err, IsNil)
	testLogger.Println("lupr:", lupr)
	c.Assert(len(lupr.UploadedParts), Equals, len(chunks))

	lmur, err := s.bucket.ListMultipartUploads()
	c.Assert(err, IsNil)
	testLogger.Println("lmur:", lmur)

	// complete
	_, err = s.bucket.CompleteMultipartUpload(imurUpload, partsUpload)
	c.Assert(err, IsNil)
	_, err = s.bucket.CompleteMultipartUpload(imurCopy, partsCopy)
	c.Assert(err, IsNil)

	// download
	err = s.bucket.GetObjectToFile(objectDesc, "newpic3.jpg")
	c.Assert(err, IsNil)
	err = s.bucket.GetObjectToFile(objectName, "newpic4.jpg")
	c.Assert(err, IsNil)

	err = s.bucket.DeleteObject(objectName)
	c.Assert(err, IsNil)
	err = s.bucket.DeleteObject(objectDesc)
	c.Assert(err, IsNil)
	err = s.bucket.DeleteObject(objectSrc)
	c.Assert(err, IsNil)
}

func (s *OssBucketMultipartSuite) TestAbortMultipartUpload(c *C) {
	objectName := objectNamePrefix + "tamu"
	objectSrc := objectNamePrefix + "tamu" + "src"
	objectDesc := objectNamePrefix + "tamu" + "desc"
	var fileName = "../sample/BingWallpaper-2015-11-07.jpg"

	chunks, err := SplitFileByPartSize(fileName, 100*1024)
	c.Assert(err, IsNil)
	testLogger.Println("chunks:", chunks)

	err = s.bucket.PutObjectFromFile(objectSrc, fileName)
	c.Assert(err, IsNil)

	// upload
	imurUpload, err := s.bucket.InitiateMultipartUpload(objectName)
	var partsUpload []UploadPart
	for _, chunk := range chunks {
		part, err := s.bucket.UploadPartFromFile(imurUpload, fileName, chunk.Offset, chunk.Size, (int)(chunk.Number))
		c.Assert(err, IsNil)
		partsUpload = append(partsUpload, part)
	}

	// copy
	imurCopy, err := s.bucket.InitiateMultipartUpload(objectDesc)
	var partsCopy []UploadPart
	for _, chunk := range chunks {
		part, err := s.bucket.UploadPartCopy(imurCopy, bucketName, objectSrc, chunk.Offset, chunk.Size, (int)(chunk.Number))
		c.Assert(err, IsNil)
		partsCopy = append(partsCopy, part)
	}

	// list
	lupr, err := s.bucket.ListUploadedParts(imurUpload)
	c.Assert(err, IsNil)
	testLogger.Println("lupr:", lupr)
	c.Assert(len(lupr.UploadedParts), Equals, len(chunks))

	lupr, err = s.bucket.ListUploadedParts(imurCopy)
	c.Assert(err, IsNil)
	testLogger.Println("lupr:", lupr)
	c.Assert(len(lupr.UploadedParts), Equals, len(chunks))

	lmur, err := s.bucket.ListMultipartUploads()
	c.Assert(err, IsNil)
	testLogger.Println("lmur:", lmur)
	c.Assert(len(lmur.Uploads), Equals, 2)

	// abort
	err = s.bucket.AbortMultipartUpload(imurUpload)
	c.Assert(err, IsNil)
	err = s.bucket.AbortMultipartUpload(imurCopy)
	c.Assert(err, IsNil)

	lmur, err = s.bucket.ListMultipartUploads()
	c.Assert(err, IsNil)
	testLogger.Println("lmur:", lmur)
	c.Assert(len(lmur.Uploads), Equals, 0)

	// download
	err = s.bucket.GetObjectToFile(objectDesc, "newpic3.jpg")
	c.Assert(err, NotNil)
	err = s.bucket.GetObjectToFile(objectName, "newpic4.jpg")
	c.Assert(err, NotNil)
}

// TestUploadPartCopyWithConstraints
func (s *OssBucketMultipartSuite) TestUploadPartCopyWithConstraints(c *C) {
	objectSrc := objectNamePrefix + "tucwc" + "src"
	objectDesc := objectNamePrefix + "tucwc" + "desc"
	var fileName = "../sample/BingWallpaper-2015-11-07.jpg"

	chunks, err := SplitFileByPartNum(fileName, 3)
	c.Assert(err, IsNil)
	testLogger.Println("chunks:", chunks)

	err = s.bucket.PutObjectFromFile(objectSrc, fileName)
	c.Assert(err, IsNil)

	imur, err := s.bucket.InitiateMultipartUpload(objectDesc)
	var parts []UploadPart
	for _, chunk := range chunks {
		_, err = s.bucket.UploadPartCopy(imur, bucketName, objectSrc, chunk.Offset, chunk.Size, (int)(chunk.Number),
			CopySourceIfModifiedSince(futureDate))
		c.Assert(err, NotNil)
	}

	for _, chunk := range chunks {
		_, err = s.bucket.UploadPartCopy(imur, bucketName, objectSrc, chunk.Offset, chunk.Size, (int)(chunk.Number),
			CopySourceIfUnmodifiedSince(futureDate))
		c.Assert(err, IsNil)
	}

	meta, err := s.bucket.GetObjectDetailedMeta(objectSrc)
	c.Assert(err, IsNil)
	testLogger.Println("GetObjectDetailedMeta:", meta)

	for _, chunk := range chunks {
		_, err = s.bucket.UploadPartCopy(imur, bucketName, objectSrc, chunk.Offset, chunk.Size, (int)(chunk.Number),
			CopySourceIfNoneMatch(meta.Get("Etag")))
		c.Assert(err, NotNil)
	}

	for _, chunk := range chunks {
		part, err := s.bucket.UploadPartCopy(imur, bucketName, objectSrc, chunk.Offset, chunk.Size, (int)(chunk.Number),
			CopySourceIfMatch(meta.Get("Etag")))
		c.Assert(err, IsNil)
		parts = append(parts, part)
	}

	cmur, err := s.bucket.CompleteMultipartUpload(imur, parts)
	c.Assert(err, IsNil)
	testLogger.Println("cmur:", cmur)

	err = s.bucket.GetObjectToFile(objectDesc, "newpic5.jpg")
	c.Assert(err, IsNil)

	err = s.bucket.DeleteObject(objectSrc)
	c.Assert(err, IsNil)
	err = s.bucket.DeleteObject(objectDesc)
	c.Assert(err, IsNil)
}

// TestMultipartUploadFromFileOutofOrder
func (s *OssBucketMultipartSuite) TestMultipartUploadFromFileOutofOrder(c *C) {
	objectName := objectNamePrefix + "tmuffoo"
	var fileName = "../sample/BingWallpaper-2015-11-07.jpg"

	chunks, err := SplitFileByPartSize(fileName, 1024*100)
	shuffleArray(chunks)
	c.Assert(err, IsNil)
	testLogger.Println("chunks:", chunks)

	imur, err := s.bucket.InitiateMultipartUpload(objectName)
	var parts []UploadPart
	for _, chunk := range chunks {
		_, err := s.bucket.UploadPartFromFile(imur, fileName, chunk.Offset, chunk.Size, (int)(chunk.Number))
		c.Assert(err, IsNil)
	}
	// double upload
	for _, chunk := range chunks {
		part, err := s.bucket.UploadPartFromFile(imur, fileName, chunk.Offset, chunk.Size, (int)(chunk.Number))
		c.Assert(err, IsNil)
		parts = append(parts, part)
	}

	cmur, err := s.bucket.CompleteMultipartUpload(imur, parts)
	c.Assert(err, IsNil)
	testLogger.Println("cmur:", cmur)

	err = s.bucket.GetObjectToFile(objectName, "newpic6.jpg")
	c.Assert(err, IsNil)

	err = s.bucket.DeleteObject(objectName)
	c.Assert(err, IsNil)
}

// TestUploadPartCopyOutofOrder
func (s *OssBucketMultipartSuite) TestUploadPartCopyOutofOrder(c *C) {
	objectSrc := objectNamePrefix + "tupcoo" + "src"
	objectDesc := objectNamePrefix + "tupcoo" + "desc"
	var fileName = "../sample/BingWallpaper-2015-11-07.jpg"

	chunks, err := SplitFileByPartSize(fileName, 1024*100)
	shuffleArray(chunks)
	c.Assert(err, IsNil)
	testLogger.Println("chunks:", chunks)

	err = s.bucket.PutObjectFromFile(objectSrc, fileName)
	c.Assert(err, IsNil)

	imur, err := s.bucket.InitiateMultipartUpload(objectDesc)
	var parts []UploadPart
	for _, chunk := range chunks {
		_, err := s.bucket.UploadPartCopy(imur, bucketName, objectSrc, chunk.Offset, chunk.Size, (int)(chunk.Number))
		c.Assert(err, IsNil)
	}
	//double copy
	for _, chunk := range chunks {
		part, err := s.bucket.UploadPartCopy(imur, bucketName, objectSrc, chunk.Offset, chunk.Size, (int)(chunk.Number))
		c.Assert(err, IsNil)
		parts = append(parts, part)
	}

	cmur, err := s.bucket.CompleteMultipartUpload(imur, parts)
	c.Assert(err, IsNil)
	testLogger.Println("cmur:", cmur)

	err = s.bucket.GetObjectToFile(objectDesc, "newpic7.jpg")
	c.Assert(err, IsNil)

	err = s.bucket.DeleteObject(objectSrc)
	c.Assert(err, IsNil)
	err = s.bucket.DeleteObject(objectDesc)
	c.Assert(err, IsNil)
}

// TestMultipartUpload
func (s *OssBucketMultipartSuite) TestMultipartUploadFromFileType(c *C) {
	objectName := objectNamePrefix + "tmuffwm" + ".jpg"
	var fileName = "../sample/BingWallpaper-2015-11-07.jpg"

	chunks, err := SplitFileByPartNum(fileName, 4)
	c.Assert(err, IsNil)
	testLogger.Println("chunks:", chunks)

	imur, err := s.bucket.InitiateMultipartUpload(objectName)
	var parts []UploadPart
	for _, chunk := range chunks {
		part, err := s.bucket.UploadPartFromFile(imur, fileName, chunk.Offset, chunk.Size, chunk.Number)
		c.Assert(err, IsNil)
		parts = append(parts, part)
	}

	testLogger.Println("parts:", parts)
	cmur, err := s.bucket.CompleteMultipartUpload(imur, parts)
	c.Assert(err, IsNil)
	testLogger.Println("cmur:", cmur)

	err = s.bucket.GetObjectToFile(objectName, "newpic8.jpg")
	c.Assert(err, IsNil)

	meta, err := s.bucket.GetObjectDetailedMeta(objectName)
	c.Assert(err, IsNil)
	c.Assert(meta.Get("Content-Type"), Equals, "image/jpeg")

	err = s.bucket.DeleteObject(objectName)
	c.Assert(err, IsNil)
}

func (s *OssBucketMultipartSuite) TestListMultipartUploads(c *C) {
	objectName := objectNamePrefix + "tlmu"

	imurs := []InitiateMultipartUploadResult{}
	for i := 0; i < 20; i++ {
		imur, err := s.bucket.InitiateMultipartUpload(objectName + strconv.Itoa(i))
		c.Assert(err, IsNil)
		imurs = append(imurs, imur)
	}

	lmpu, err := s.bucket.ListMultipartUploads()
	c.Assert(err, IsNil)
	c.Assert(len(lmpu.Uploads), Equals, 20)

	lmpu, err = s.bucket.ListMultipartUploads(MaxUploads(3))
	c.Assert(err, IsNil)
	c.Assert(len(lmpu.Uploads), Equals, 3)

	lmpu, err = s.bucket.ListMultipartUploads(Prefix(objectName))
	c.Assert(err, IsNil)
	c.Assert(len(lmpu.Uploads), Equals, 20)

	lmpu, err = s.bucket.ListMultipartUploads(Prefix(objectName + "1"))
	c.Assert(err, IsNil)
	c.Assert(len(lmpu.Uploads), Equals, 11)

	lmpu, err = s.bucket.ListMultipartUploads(Prefix(objectName + "22"))
	c.Assert(err, IsNil)
	c.Assert(len(lmpu.Uploads), Equals, 0)

	lmpu, err = s.bucket.ListMultipartUploads(KeyMarker(objectName + "10"))
	c.Assert(err, IsNil)
	c.Assert(len(lmpu.Uploads), Equals, 17)

	lmpu, err = s.bucket.ListMultipartUploads(KeyMarker(objectName+"10"), MaxUploads(3))
	c.Assert(err, IsNil)
	c.Assert(len(lmpu.Uploads), Equals, 3)

	lmpu, err = s.bucket.ListMultipartUploads(Prefix(objectName), Delimiter("4"))
	c.Assert(err, IsNil)
	c.Assert(len(lmpu.Uploads), Equals, 18)
	c.Assert(len(lmpu.CommonPrefixes), Equals, 2)

	// upload-id-marker
	lmpu, err = s.bucket.ListMultipartUploads(KeyMarker(objectName+"12"), UploadIDMarker("EEE"))
	c.Assert(err, IsNil)
	c.Assert(len(lmpu.Uploads), Equals, 15)
	//testLogger.Println("UploadIDMarker", lmpu.Uploads)

	for _, imur := range imurs {
		err = s.bucket.AbortMultipartUpload(imur)
		c.Assert(err, IsNil)
	}
}

func (s *OssBucketMultipartSuite) TestListMultipartUploadsEncodingKey(c *C) {
	objectName := objectNamePrefix + "让你任性让你狂" + "tlmuek"

	imurs := []InitiateMultipartUploadResult{}
	for i := 0; i < 3; i++ {
		imur, err := s.bucket.InitiateMultipartUpload(objectName + strconv.Itoa(i))
		c.Assert(err, IsNil)
		imurs = append(imurs, imur)
	}

	lmpu, err := s.bucket.ListMultipartUploads()
	c.Assert(err, IsNil)
	c.Assert(len(lmpu.Uploads), Equals, 3)

	lmpu, err = s.bucket.ListMultipartUploads(Prefix(objectNamePrefix + "让你任性让你狂tlmuek1"))
	c.Assert(err, IsNil)
	c.Assert(len(lmpu.Uploads), Equals, 1)

	lmpu, err = s.bucket.ListMultipartUploads(KeyMarker(objectNamePrefix + "让你任性让你狂tlmuek1"))
	c.Assert(err, IsNil)
	c.Assert(len(lmpu.Uploads), Equals, 1)

	lmpu, err = s.bucket.ListMultipartUploads(EncodingType("url"))
	c.Assert(err, IsNil)
	for i, upload := range lmpu.Uploads {
		c.Assert(upload.Key, Equals, objectNamePrefix+"让你任性让你狂tlmuek"+strconv.Itoa(i))
	}

	for _, imur := range imurs {
		err = s.bucket.AbortMultipartUpload(imur)
		c.Assert(err, IsNil)
	}
}

func (s *OssBucketMultipartSuite) TestMultipartNegative(c *C) {
	objectName := objectNamePrefix + "tmn"

	// key tool long
	data := make([]byte, 100*1024)
	imur, err := s.bucket.InitiateMultipartUpload(string(data))
	c.Assert(err, NotNil)

	// imur invalid
	fileName := "../sample/BingWallpaper-2015-11-07.jpg"
	fd, err := os.Open(fileName)
	c.Assert(err, IsNil)
	defer fd.Close()

	_, err = s.bucket.UploadPart(imur, fd, 1024, 1)
	c.Assert(err, NotNil)

	_, err = s.bucket.UploadPartFromFile(imur, fileName, 0, 1024, 1)
	c.Assert(err, NotNil)

	_, err = s.bucket.UploadPartCopy(imur, bucketName, fileName, 0, 1024, 1)
	c.Assert(err, NotNil)

	err = s.bucket.AbortMultipartUpload(imur)
	c.Assert(err, NotNil)

	_, err = s.bucket.ListUploadedParts(imur)
	c.Assert(err, NotNil)

	// invalid exist
	imur, err = s.bucket.InitiateMultipartUpload(objectName)
	c.Assert(err, IsNil)

	_, err = s.bucket.UploadPart(imur, fd, 1024, 1)
	c.Assert(err, IsNil)

	_, err = s.bucket.UploadPart(imur, fd, 102400, 10001)
	c.Assert(err, NotNil)

	//    _, err = s.bucket.UploadPartFromFile(imur, fileName, 0, 1024, 1)
	//    c.Assert(err, IsNil)

	_, err = s.bucket.UploadPartFromFile(imur, fileName, 0, 102400, 10001)
	c.Assert(err, NotNil)

	_, err = s.bucket.UploadPartCopy(imur, bucketName, fileName, 0, 1024, 1)
	c.Assert(err, NotNil)

	_, err = s.bucket.UploadPartCopy(imur, bucketName, fileName, 0, 1024, 1000)
	c.Assert(err, NotNil)

	err = s.bucket.AbortMultipartUpload(imur)
	c.Assert(err, IsNil)

	// option invalid
	_, err = s.bucket.InitiateMultipartUpload(objectName, IfModifiedSince(futureDate))
	c.Assert(err, IsNil)
}

func (s *OssBucketMultipartSuite) TestMultipartUploadFromFileBigFile(c *C) {
	objectName := objectNamePrefix + "tmuffbf"
	bigFile := "D:\\tmp\\bigfile.zip"
	newFile := "D:\\tmp\\newbigfile.zip"

	exist, err := isFileExist(bigFile)
	c.Assert(err, IsNil)
	if !exist {
		return
	}

	chunks, err := SplitFileByPartNum(bigFile, 64)
	c.Assert(err, IsNil)
	testLogger.Println("chunks:", chunks)

	imur, err := s.bucket.InitiateMultipartUpload(objectName)
	var parts []UploadPart
	start := GetNowSec()
	for _, chunk := range chunks {
		part, err := s.bucket.UploadPartFromFile(imur, bigFile, chunk.Offset, chunk.Size, (int)(chunk.Number))
		c.Assert(err, IsNil)
		parts = append(parts, part)
	}
	end := GetNowSec()
	testLogger.Println("Uplaod big file:", bigFile, "use sec:", end-start)

	testLogger.Println("parts:", parts)
	_, err = s.bucket.CompleteMultipartUpload(imur, parts)
	c.Assert(err, IsNil)

	start = GetNowSec()
	err = s.bucket.GetObjectToFile(objectName, newFile)
	c.Assert(err, IsNil)
	end = GetNowSec()
	testLogger.Println("Download big file:", bigFile, "use sec:", end-start)

	start = GetNowSec()
	eq, err := compareFiles(bigFile, newFile)
	c.Assert(err, IsNil)
	c.Assert(eq, Equals, true)
	end = GetNowSec()
	testLogger.Println("Compare big file:", bigFile, "use sec:", end-start)

	err = s.bucket.DeleteObject(objectName)
	c.Assert(err, IsNil)
}

// TestUploadFile
func (s *OssBucketMultipartSuite) TestUploadFile(c *C) {
	objectName := objectNamePrefix + "tuff"
	var fileName = "../sample/BingWallpaper-2015-11-07.jpg"
	newFile := "newfiletuff.jpg"

	// 有余数
	err := s.bucket.UploadFile(objectName, fileName, 100*1024)
	c.Assert(err, IsNil)

	os.Remove(newFile)
	err = s.bucket.GetObjectToFile(objectName, newFile)
	c.Assert(err, IsNil)

	eq, err := compareFiles(fileName, newFile)
	c.Assert(err, IsNil)
	c.Assert(eq, Equals, true)

	err = s.bucket.DeleteObject(objectName)
	c.Assert(err, IsNil)

	// 整除
	err = s.bucket.UploadFile(objectName, fileName, 482048/4)
	c.Assert(err, IsNil)

	os.Remove(newFile)
	err = s.bucket.GetObjectToFile(objectName, newFile)
	c.Assert(err, IsNil)

	eq, err = compareFiles(fileName, newFile)
	c.Assert(err, IsNil)
	c.Assert(eq, Equals, true)

	err = s.bucket.DeleteObject(objectName)
	c.Assert(err, IsNil)

	// 等于文件大小
	err = s.bucket.UploadFile(objectName, fileName, 482048)
	c.Assert(err, IsNil)

	os.Remove(newFile)
	err = s.bucket.GetObjectToFile(objectName, newFile)
	c.Assert(err, IsNil)

	eq, err = compareFiles(fileName, newFile)
	c.Assert(err, IsNil)
	c.Assert(eq, Equals, true)

	err = s.bucket.DeleteObject(objectName)
	c.Assert(err, IsNil)

	// 大于文件大小
	err = s.bucket.UploadFile(objectName, fileName, 482049)
	c.Assert(err, IsNil)

	os.Remove(newFile)
	err = s.bucket.GetObjectToFile(objectName, newFile)
	c.Assert(err, IsNil)

	eq, err = compareFiles(fileName, newFile)
	c.Assert(err, IsNil)
	c.Assert(eq, Equals, true)

	err = s.bucket.DeleteObject(objectName)
	c.Assert(err, IsNil)

	// option
	options := []Option{
		Expires(futureDate),
		ObjectACL(ACLPublicRead),
		Meta("myprop", "mypropval")}
	err = s.bucket.UploadFile(objectName, fileName, 482049, options...)
	c.Assert(err, IsNil)

	// Check
	os.Remove(newFile)
	err = s.bucket.GetObjectToFile(objectName, newFile)
	c.Assert(err, IsNil)

	eq, err = compareFiles(fileName, newFile)
	c.Assert(err, IsNil)
	c.Assert(eq, Equals, true)

	acl, err := s.bucket.GetObjectACL(objectName)
	c.Assert(err, IsNil)
	testLogger.Println("GetObjectAcl:", acl)
	c.Assert(acl.ACL, Equals, "default")

	meta, err := s.bucket.GetObjectDetailedMeta(objectName)
	c.Assert(err, IsNil)
	testLogger.Println("GetObjectDetailedMeta:", meta)
	c.Assert(meta.Get("X-Oss-Meta-Myprop"), Equals, "mypropval")
}

func (s *OssBucketMultipartSuite) TestUploadFileNegative(c *C) {
	objectName := objectNamePrefix + "tufn"
	var fileName = "../sample/BingWallpaper-2015-11-07.jpg"

	// 小于最小文件片
	err := s.bucket.UploadFile(objectName, fileName, 100*1024-1)
	c.Assert(err, NotNil)

	// 大于最大文件片
	err = s.bucket.UploadFile(objectName, fileName, 1024*1024*1024*5+1)
	c.Assert(err, NotNil)

	// 文件不存在
	err = s.bucket.UploadFile(objectName, "/root/123abc9874", 1024*1024*1024)
	c.Assert(err, NotNil)

	// Key无效
	err = s.bucket.UploadFile("", fileName, 100*1024)
	c.Assert(err, NotNil)
}

// TestDownloadFile
func (s *OssBucketMultipartSuite) TestDownloadFile(c *C) {
	objectName := objectNamePrefix + "tdff"
	var fileName = "../sample/BingWallpaper-2015-11-07.jpg"
	newFile := "newfiletdff.jpg"

	err := s.bucket.UploadFile(objectName, fileName, 100*1024)
	c.Assert(err, IsNil)

	// 有余数
	err = s.bucket.DownloadFile(objectName, newFile, 100*1024)
	c.Assert(err, IsNil)

	os.Remove(newFile)
	err = s.bucket.GetObjectToFile(objectName, newFile)
	c.Assert(err, IsNil)

	eq, err := compareFiles(fileName, newFile)
	c.Assert(err, IsNil)
	c.Assert(eq, Equals, true)

	// 整除
	err = s.bucket.DownloadFile(objectName, newFile, 482048/4)
	c.Assert(err, IsNil)

	os.Remove(newFile)
	err = s.bucket.GetObjectToFile(objectName, newFile)
	c.Assert(err, IsNil)

	eq, err = compareFiles(fileName, newFile)
	c.Assert(err, IsNil)
	c.Assert(eq, Equals, true)

	// 等于文件大小
	err = s.bucket.DownloadFile(objectName, newFile, 482048)
	c.Assert(err, IsNil)

	os.Remove(newFile)
	err = s.bucket.GetObjectToFile(objectName, newFile)
	c.Assert(err, IsNil)

	eq, err = compareFiles(fileName, newFile)
	c.Assert(err, IsNil)
	c.Assert(eq, Equals, true)

	// 大于文件大小
	err = s.bucket.DownloadFile(objectName, newFile, 482049)
	c.Assert(err, IsNil)

	os.Remove(newFile)
	err = s.bucket.GetObjectToFile(objectName, newFile)
	c.Assert(err, IsNil)

	eq, err = compareFiles(fileName, newFile)
	c.Assert(err, IsNil)
	c.Assert(eq, Equals, true)

	// option
	meta, err := s.bucket.GetObjectDetailedMeta(objectName)
	c.Assert(err, IsNil)
	testLogger.Println("GetObjectDetailedMeta:", meta)

	// If-Match
	err = s.bucket.DownloadFile(objectName, newFile, 482048/4, IfMatch(meta.Get("Etag")))
	c.Assert(err, IsNil)

	os.Remove(newFile)
	err = s.bucket.GetObjectToFile(objectName, newFile)
	c.Assert(err, IsNil)

	eq, err = compareFiles(fileName, newFile)
	c.Assert(err, IsNil)
	c.Assert(eq, Equals, true)

	// If-None-Match
	err = s.bucket.DownloadFile(objectName, newFile, 482048, IfNoneMatch(meta.Get("Etag")))
	c.Assert(err, NotNil)

	os.Remove(newFile)
	err = s.bucket.DeleteObject(objectName)
	c.Assert(err, IsNil)
}

func (s *OssBucketMultipartSuite) TestDownloadFileNegative(c *C) {
	objectName := objectNamePrefix + "tufn"
	newFile := "newfiletudff.jpg"

	// 小于最小文件片
	err := s.bucket.DownloadFile(objectName, newFile, 100*1024-1)
	c.Assert(err, NotNil)

	// 大于最大文件片
	err = s.bucket.DownloadFile(objectName, newFile, 1024*1024*1024+1)
	c.Assert(err, NotNil)

	// 文件不存在
	err = s.bucket.DownloadFile(objectName, "/OSS/TEMP/ZIBI/QUQU/BALA", 1024*1024*1024+1)
	c.Assert(err, NotNil)

	// Key不存在
	err = s.bucket.DownloadFile(objectName, newFile, 100*1024)
	c.Assert(err, NotNil)
}

// private
func shuffleArray(chunks []FileChunk) []FileChunk {
	for i := range chunks {
		j := rand.Intn(i + 1)
		chunks[i], chunks[j] = chunks[j], chunks[i]
	}
	return chunks
}
