// bucket test

package oss

import (
	"bytes"
	"io/ioutil"
	"math/rand"
	"os"
	"strings"
	"time"

	. "gopkg.in/check.v1"
)

type OssProgressSuite struct {
	client *Client
	bucket *Bucket
}

var _ = Suite(&OssProgressSuite{})

// Run once when the suite starts running
func (s *OssProgressSuite) SetUpSuite(c *C) {
	client, err := New(endpoint, accessID, accessKey)
	c.Assert(err, IsNil)
	s.client = client

	s.client.CreateBucket(bucketName)
	time.Sleep(5 * time.Second)

	bucket, err := s.client.Bucket(bucketName)
	c.Assert(err, IsNil)
	s.bucket = bucket

	testLogger.Println("test progress started")
}

// Run before each test or benchmark starts running
func (s *OssProgressSuite) TearDownSuite(c *C) {
	// Delete Multipart
	lmu, err := s.bucket.ListMultipartUploads()
	c.Assert(err, IsNil)

	for _, upload := range lmu.Uploads {
		imur := InitiateMultipartUploadResult{Bucket: bucketName, Key: upload.Key, UploadID: upload.UploadID}
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

	testLogger.Println("test progress completed")
}

// Run after each test or benchmark runs
func (s *OssProgressSuite) SetUpTest(c *C) {
	err := removeTempFiles("../oss", ".jpg")
	c.Assert(err, IsNil)

	err = removeTempFiles("../oss", ".txt")
	c.Assert(err, IsNil)

	err = removeTempFiles("../oss", ".html")
	c.Assert(err, IsNil)
}

// Run once after all tests or benchmarks have finished running
func (s *OssProgressSuite) TearDownTest(c *C) {
	err := removeTempFiles("../oss", ".jpg")
	c.Assert(err, IsNil)

	err = removeTempFiles("../oss", ".txt")
	c.Assert(err, IsNil)

	err = removeTempFiles("../oss", ".html")
	c.Assert(err, IsNil)
}

// OssProgressListener progress listener
type OssProgressListener struct {
}

// ProgressChanged handle progress event
func (listener *OssProgressListener) ProgressChanged(event *ProgressEvent) {
	switch event.EventType {
	case TransferStartedEvent:
		testLogger.Printf("Transfer Started, ConsumedBytes: %d, TotalBytes %d.\n",
			event.ConsumedBytes, event.TotalBytes)
	case TransferDataEvent:
		testLogger.Printf("Transfer Data, ConsumedBytes: %d, TotalBytes %d, %d%%.\n",
			event.ConsumedBytes, event.TotalBytes, event.ConsumedBytes*100/event.TotalBytes)
	case TransferCompletedEvent:
		testLogger.Printf("Transfer Completed, ConsumedBytes: %d, TotalBytes %d.\n",
			event.ConsumedBytes, event.TotalBytes)
	case TransferFailedEvent:
		testLogger.Printf("Transfer Failed, ConsumedBytes: %d, TotalBytes %d.\n",
			event.ConsumedBytes, event.TotalBytes)
	default:
	}
}

// TestPutObject
func (s *OssProgressSuite) TestPutObject(c *C) {
	objectName := objectNamePrefix + "tpo.html"
	localFile := "../sample/The Go Programming Language.html"

	// PutObject
	fd, err := os.Open(localFile)
	c.Assert(err, IsNil)
	defer fd.Close()

	err = s.bucket.PutObject(objectName, fd, Progress(&OssProgressListener{}))
	c.Assert(err, IsNil)

	// PutObjectFromFile
	err = s.bucket.PutObjectFromFile(objectName, localFile, Progress(&OssProgressListener{}))
	c.Assert(err, IsNil)

	// DoPutObject
	fd, err = os.Open(localFile)
	c.Assert(err, IsNil)
	defer fd.Close()

	request := &PutObjectRequest{
		ObjectKey: objectName,
		Reader:    fd,
	}

	options := []Option{Progress(&OssProgressListener{})}
	_, err = s.bucket.DoPutObject(request, options)
	c.Assert(err, IsNil)

	// PutObject size is 0
	err = s.bucket.PutObject(objectName, strings.NewReader(""), Progress(&OssProgressListener{}))
	c.Assert(err, IsNil)

	testLogger.Println("OssProgressSuite.TestPutObject")
}

// Test SignURL
func (s *OssProgressSuite) TestSignURL(c *C) {
	objectName := objectNamePrefix + randStr(5)
	filePath := randLowStr(10)
	content := randStr(20)
	createFile(filePath, content, c)

	// sign url for put
	str, err := s.bucket.SignURL(objectName, HTTPPut, 60, Progress(&OssProgressListener{}))
	c.Assert(err, IsNil)
	c.Assert(strings.Contains(str, HTTPParamExpires+"="), Equals, true)
	c.Assert(strings.Contains(str, HTTPParamAccessKeyID+"="), Equals, true)
	c.Assert(strings.Contains(str, HTTPParamSignature+"="), Equals, true)

	// put object with url
	fd, err := os.Open(filePath)
	c.Assert(err, IsNil)
	defer fd.Close()

	err = s.bucket.PutObjectWithURL(str, fd, Progress(&OssProgressListener{}))
	c.Assert(err, IsNil)

	// put object from file with url
	err = s.bucket.PutObjectFromFileWithURL(str, filePath, Progress(&OssProgressListener{}))
	c.Assert(err, IsNil)

	// DoPutObject
	fd, err = os.Open(filePath)
	c.Assert(err, IsNil)
	defer fd.Close()

	options := []Option{Progress(&OssProgressListener{})}
	_, err = s.bucket.DoPutObjectWithURL(str, fd, options)
	c.Assert(err, IsNil)

	// sign url for get
	str, err = s.bucket.SignURL(objectName, HTTPGet, 60, Progress(&OssProgressListener{}))
	c.Assert(err, IsNil)
	c.Assert(strings.Contains(str, HTTPParamExpires+"="), Equals, true)
	c.Assert(strings.Contains(str, HTTPParamAccessKeyID+"="), Equals, true)
	c.Assert(strings.Contains(str, HTTPParamSignature+"="), Equals, true)

	// get object with url
	body, err := s.bucket.GetObjectWithURL(str, Progress(&OssProgressListener{}))
	c.Assert(err, IsNil)
	str, err = readBody(body)
	c.Assert(err, IsNil)
	c.Assert(str, Equals, content)

	// get object to file with url
	str, err = s.bucket.SignURL(objectName, HTTPGet, 10, Progress(&OssProgressListener{}))
	c.Assert(err, IsNil)

	newFile := randStr(10)
	err = s.bucket.GetObjectToFileWithURL(str, newFile, Progress(&OssProgressListener{}))
	c.Assert(err, IsNil)
	eq, err := compareFiles(filePath, newFile)
	c.Assert(err, IsNil)
	c.Assert(eq, Equals, true)

	os.Remove(filePath)
	os.Remove(newFile)

	err = s.bucket.DeleteObject(objectName)
	c.Assert(err, IsNil)

	testLogger.Println("OssProgressSuite.TestSignURL")
}

func (s *OssProgressSuite) TestPutObjectNegative(c *C) {
	objectName := objectNamePrefix + "tpon.html"
	localFile := "../sample/The Go Programming Language.html"

	// invalid endpoint
	client, err := New("http://oss-cn-taikang.aliyuncs.com", accessID, accessKey)
	c.Assert(err, IsNil)

	bucket, err := client.Bucket(bucketName)
	c.Assert(err, IsNil)

	err = bucket.PutObjectFromFile(objectName, localFile, Progress(&OssProgressListener{}))
	testLogger.Println(err)
	c.Assert(err, NotNil)

	testLogger.Println("OssProgressSuite.TestPutObjectNegative")
}

// TestAppendObject
func (s *OssProgressSuite) TestAppendObject(c *C) {
	objectName := objectNamePrefix + "tao"
	objectValue := "昨夜雨疏风骤，浓睡不消残酒。试问卷帘人，却道海棠依旧。知否？知否？应是绿肥红瘦。"
	var val = []byte(objectValue)
	var nextPos int64
	var midPos = 1 + rand.Intn(len(val)-1)

	// AppendObject
	nextPos, err := s.bucket.AppendObject(objectName, bytes.NewReader(val[0:midPos]), nextPos, Progress(&OssProgressListener{}))
	c.Assert(err, IsNil)

	// DoAppendObject
	request := &AppendObjectRequest{
		ObjectKey: objectName,
		Reader:    bytes.NewReader(val[midPos:]),
		Position:  nextPos,
	}
	options := []Option{Progress(&OssProgressListener{})}
	_, err = s.bucket.DoAppendObject(request, options)
	c.Assert(err, IsNil)

	testLogger.Println("OssProgressSuite.TestAppendObject")
}

// TestMultipartUpload
func (s *OssProgressSuite) TestMultipartUpload(c *C) {
	objectName := objectNamePrefix + "tmu.jpg"
	var fileName = "../sample/BingWallpaper-2015-11-07.jpg"

	chunks, err := SplitFileByPartNum(fileName, 3)
	c.Assert(err, IsNil)
	testLogger.Println("chunks:", chunks)

	fd, err := os.Open(fileName)
	c.Assert(err, IsNil)
	defer fd.Close()

	// Initiate
	imur, err := s.bucket.InitiateMultipartUpload(objectName)
	c.Assert(err, IsNil)

	// UploadPart
	var parts []UploadPart
	for _, chunk := range chunks {
		fd.Seek(chunk.Offset, os.SEEK_SET)
		part, err := s.bucket.UploadPart(imur, fd, chunk.Size, chunk.Number, Progress(&OssProgressListener{}))
		c.Assert(err, IsNil)
		parts = append(parts, part)
	}

	// Complete
	_, err = s.bucket.CompleteMultipartUpload(imur, parts)
	c.Assert(err, IsNil)

	err = s.bucket.DeleteObject(objectName)
	c.Assert(err, IsNil)

	testLogger.Println("OssProgressSuite.TestMultipartUpload")
}

// TestMultipartUploadFromFile
func (s *OssProgressSuite) TestMultipartUploadFromFile(c *C) {
	objectName := objectNamePrefix + "tmuff.jpg"
	var fileName = "../sample/BingWallpaper-2015-11-07.jpg"

	chunks, err := SplitFileByPartNum(fileName, 3)
	c.Assert(err, IsNil)

	// Initiate
	imur, err := s.bucket.InitiateMultipartUpload(objectName)
	c.Assert(err, IsNil)

	// UploadPart
	var parts []UploadPart
	for _, chunk := range chunks {
		part, err := s.bucket.UploadPartFromFile(imur, fileName, chunk.Offset, chunk.Size, chunk.Number, Progress(&OssProgressListener{}))
		c.Assert(err, IsNil)
		parts = append(parts, part)
	}

	// Complete
	_, err = s.bucket.CompleteMultipartUpload(imur, parts)
	c.Assert(err, IsNil)

	err = s.bucket.DeleteObject(objectName)
	c.Assert(err, IsNil)

	testLogger.Println("OssProgressSuite.TestMultipartUploadFromFile")
}

// TestGetObject
func (s *OssProgressSuite) TestGetObject(c *C) {
	objectName := objectNamePrefix + "tgo.jpg"
	localFile := "../sample/BingWallpaper-2015-11-07.jpg"
	newFile := "newpic-progress-1.jpg"

	// PutObject
	err := s.bucket.PutObjectFromFile(objectName, localFile, Progress(&OssProgressListener{}))
	c.Assert(err, IsNil)

	// GetObject
	body, err := s.bucket.GetObject(objectName, Progress(&OssProgressListener{}))
	c.Assert(err, IsNil)
	_, err = ioutil.ReadAll(body)
	c.Assert(err, IsNil)
	body.Close()

	// GetObjectToFile
	err = s.bucket.GetObjectToFile(objectName, newFile, Progress(&OssProgressListener{}))
	c.Assert(err, IsNil)

	// DoGetObject
	request := &GetObjectRequest{objectName}
	options := []Option{Progress(&OssProgressListener{})}
	result, err := s.bucket.DoGetObject(request, options)
	c.Assert(err, IsNil)
	_, err = ioutil.ReadAll(result.Response.Body)
	c.Assert(err, IsNil)
	result.Response.Body.Close()

	// GetObject with range
	body, err = s.bucket.GetObject(objectName, Range(1024, 4*1024), Progress(&OssProgressListener{}))
	c.Assert(err, IsNil)
	_, err = ioutil.ReadAll(body)
	c.Assert(err, IsNil)
	body.Close()

	// PutObject size is 0
	err = s.bucket.PutObject(objectName, strings.NewReader(""), Progress(&OssProgressListener{}))
	c.Assert(err, IsNil)

	// GetObject size is 0
	body, err = s.bucket.GetObject(objectName, Progress(&OssProgressListener{}))
	c.Assert(err, IsNil)
	_, err = ioutil.ReadAll(body)
	c.Assert(err, IsNil)
	body.Close()

	testLogger.Println("OssProgressSuite.TestGetObject")
}

// TestGetObjectNegative
func (s *OssProgressSuite) TestGetObjectNegative(c *C) {
	objectName := objectNamePrefix + "tgon.jpg"
	localFile := "../sample/BingWallpaper-2015-11-07.jpg"

	// PutObject
	err := s.bucket.PutObjectFromFile(objectName, localFile)
	c.Assert(err, IsNil)

	// GetObject
	body, err := s.bucket.GetObject(objectName, Progress(&OssProgressListener{}))
	c.Assert(err, IsNil)

	buf := make([]byte, 4*1024)
	n, err := body.Read(buf)
	c.Assert(err, IsNil)

	//time.Sleep(70 * time.Second) TODO

	// read should fail
	for err == nil {
		n, err = body.Read(buf)
		n += n
	}
	c.Assert(err, NotNil)
	body.Close()

	testLogger.Println("OssProgressSuite.TestGetObjectNegative")
}

// TestUploadFile
func (s *OssProgressSuite) TestUploadFile(c *C) {
	objectName := objectNamePrefix + "tuf.jpg"
	fileName := "../sample/BingWallpaper-2015-11-07.jpg"

	err := s.bucket.UploadFile(objectName, fileName, 100*1024, Routines(5), Progress(&OssProgressListener{}))
	c.Assert(err, IsNil)

	err = s.bucket.UploadFile(objectName, fileName, 100*1024, Routines(3), Checkpoint(true, objectName+".cp"), Progress(&OssProgressListener{}))
	c.Assert(err, IsNil)

	testLogger.Println("OssProgressSuite.TestUploadFile")
}

// TestDownloadFile
func (s *OssProgressSuite) TestDownloadFile(c *C) {
	objectName := objectNamePrefix + "tdf.jpg"
	fileName := "../sample/BingWallpaper-2015-11-07.jpg"
	newFile := "down-new-file-progress-2.jpg"

	// upload
	err := s.bucket.UploadFile(objectName, fileName, 100*1024, Routines(3))
	c.Assert(err, IsNil)

	err = s.bucket.DownloadFile(objectName, newFile, 100*1024, Routines(5), Progress(&OssProgressListener{}))
	c.Assert(err, IsNil)

	err = s.bucket.DownloadFile(objectName, newFile, 1024*1024, Routines(3), Progress(&OssProgressListener{}))
	c.Assert(err, IsNil)

	err = s.bucket.DownloadFile(objectName, newFile, 50*1024, Routines(3), Checkpoint(true, ""), Progress(&OssProgressListener{}))
	c.Assert(err, IsNil)

	testLogger.Println("OssProgressSuite.TestDownloadFile")
}

// TestCopyFile
func (s *OssProgressSuite) TestCopyFile(c *C) {
	srcObjectName := objectNamePrefix + "tcf.jpg"
	destObjectName := srcObjectName + "-copy"
	fileName := "../sample/BingWallpaper-2015-11-07.jpg"

	// upload
	err := s.bucket.UploadFile(srcObjectName, fileName, 100*1024, Routines(3))
	c.Assert(err, IsNil)

	err = s.bucket.CopyFile(bucketName, srcObjectName, destObjectName, 100*1024, Routines(5), Progress(&OssProgressListener{}))
	c.Assert(err, IsNil)

	err = s.bucket.CopyFile(bucketName, srcObjectName, destObjectName, 1024*100, Routines(3), Checkpoint(true, ""), Progress(&OssProgressListener{}))
	c.Assert(err, IsNil)

	testLogger.Println("OssProgressSuite.TestCopyFile")
}
