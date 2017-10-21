package oss

import (
	"fmt"
	"io"
	"os"
	"time"

	. "gopkg.in/check.v1"
)

type OssUploadSuite struct {
	client *Client
	bucket *Bucket
}

var _ = Suite(&OssUploadSuite{})

// Run once when the suite starts running
func (s *OssUploadSuite) SetUpSuite(c *C) {
	client, err := New(endpoint, accessID, accessKey)
	c.Assert(err, IsNil)
	s.client = client

	s.client.CreateBucket(bucketName)
	time.Sleep(5 * time.Second)

	bucket, err := s.client.Bucket(bucketName)
	c.Assert(err, IsNil)
	s.bucket = bucket

	testLogger.Println("test upload started")
}

// Run before each test or benchmark starts running
func (s *OssUploadSuite) TearDownSuite(c *C) {
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

	testLogger.Println("test upload completed")
}

// Run after each test or benchmark runs
func (s *OssUploadSuite) SetUpTest(c *C) {
	err := removeTempFiles("../oss", ".jpg")
	c.Assert(err, IsNil)
}

// Run once after all tests or benchmarks have finished running
func (s *OssUploadSuite) TearDownTest(c *C) {
	err := removeTempFiles("../oss", ".jpg")
	c.Assert(err, IsNil)
}

// TestUploadRoutineWithoutRecovery 多线程无断点恢复的上传
func (s *OssUploadSuite) TestUploadRoutineWithoutRecovery(c *C) {
	objectName := objectNamePrefix + "turwr"
	fileName := "../sample/BingWallpaper-2015-11-07.jpg"
	newFile := "upload-new-file.jpg"

	// 不指定Routines，默认单线程
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

	// 指定线程数1
	err = s.bucket.UploadFile(objectName, fileName, 100*1024, Routines(1))
	c.Assert(err, IsNil)

	os.Remove(newFile)
	err = s.bucket.GetObjectToFile(objectName, newFile)
	c.Assert(err, IsNil)

	eq, err = compareFiles(fileName, newFile)
	c.Assert(err, IsNil)
	c.Assert(eq, Equals, true)

	err = s.bucket.DeleteObject(objectName)
	c.Assert(err, IsNil)

	// 指定线程数3，小于分片数5
	err = s.bucket.UploadFile(objectName, fileName, 100*1024, Routines(3))
	c.Assert(err, IsNil)

	os.Remove(newFile)
	err = s.bucket.GetObjectToFile(objectName, newFile)
	c.Assert(err, IsNil)

	eq, err = compareFiles(fileName, newFile)
	c.Assert(err, IsNil)
	c.Assert(eq, Equals, true)

	err = s.bucket.DeleteObject(objectName)
	c.Assert(err, IsNil)

	// 指定线程数5，等于分片数
	err = s.bucket.UploadFile(objectName, fileName, 100*1024, Routines(5))
	c.Assert(err, IsNil)

	os.Remove(newFile)
	err = s.bucket.GetObjectToFile(objectName, newFile)
	c.Assert(err, IsNil)

	eq, err = compareFiles(fileName, newFile)
	c.Assert(err, IsNil)
	c.Assert(eq, Equals, true)

	err = s.bucket.DeleteObject(objectName)
	c.Assert(err, IsNil)

	// 指定线程数10，大于分片数5
	err = s.bucket.UploadFile(objectName, fileName, 100*1024, Routines(10))
	c.Assert(err, IsNil)

	os.Remove(newFile)
	err = s.bucket.GetObjectToFile(objectName, newFile)
	c.Assert(err, IsNil)

	eq, err = compareFiles(fileName, newFile)
	c.Assert(err, IsNil)
	c.Assert(eq, Equals, true)

	err = s.bucket.DeleteObject(objectName)
	c.Assert(err, IsNil)

	// 线程值无效自动变成1
	err = s.bucket.UploadFile(objectName, fileName, 100*1024, Routines(0))
	os.Remove(newFile)
	err = s.bucket.GetObjectToFile(objectName, newFile)
	c.Assert(err, IsNil)

	eq, err = compareFiles(fileName, newFile)
	c.Assert(err, IsNil)
	c.Assert(eq, Equals, true)

	err = s.bucket.DeleteObject(objectName)
	c.Assert(err, IsNil)

	// 线程值无效自动变成1
	err = s.bucket.UploadFile(objectName, fileName, 100*1024, Routines(-1))
	os.Remove(newFile)
	err = s.bucket.GetObjectToFile(objectName, newFile)
	c.Assert(err, IsNil)

	eq, err = compareFiles(fileName, newFile)
	c.Assert(err, IsNil)
	c.Assert(eq, Equals, true)

	err = s.bucket.DeleteObject(objectName)
	c.Assert(err, IsNil)

	// option
	err = s.bucket.UploadFile(objectName, fileName, 100*1024, Routines(3), Meta("myprop", "mypropval"))

	meta, err := s.bucket.GetObjectDetailedMeta(objectName)
	c.Assert(err, IsNil)
	c.Assert(meta.Get("X-Oss-Meta-Myprop"), Equals, "mypropval")

	os.Remove(newFile)
	err = s.bucket.GetObjectToFile(objectName, newFile)
	c.Assert(err, IsNil)

	eq, err = compareFiles(fileName, newFile)
	c.Assert(err, IsNil)
	c.Assert(eq, Equals, true)

	err = s.bucket.DeleteObject(objectName)
	c.Assert(err, IsNil)
}

// ErrorHooker UploadPart请求Hook
func ErrorHooker(id int, chunk FileChunk) error {
	if chunk.Number == 5 {
		time.Sleep(time.Second)
		return fmt.Errorf("ErrorHooker")
	}
	return nil
}

// TestUploadRoutineWithoutRecovery 多线程无断点恢复的上传
func (s *OssUploadSuite) TestUploadRoutineWithoutRecoveryNegative(c *C) {
	objectName := objectNamePrefix + "turwrn"
	fileName := "../sample/BingWallpaper-2015-11-07.jpg"

	uploadPartHooker = ErrorHooker
	// worker线程错误
	err := s.bucket.UploadFile(objectName, fileName, 100*1024, Routines(2))
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "ErrorHooker")
	uploadPartHooker = defaultUploadPart

	// 本地文件不存在
	err = s.bucket.UploadFile(objectName, "NotExist", 100*1024, Routines(2))
	c.Assert(err, NotNil)

	// 指定的分片大小无效
	err = s.bucket.UploadFile(objectName, fileName, 1024, Routines(2))
	c.Assert(err, NotNil)

	err = s.bucket.UploadFile(objectName, fileName, 1024*1024*1024*100, Routines(2))
	c.Assert(err, NotNil)
}

// TestUploadRoutineWithRecovery 多线程且有断点恢复的上传
func (s *OssUploadSuite) TestUploadRoutineWithRecovery(c *C) {
	objectName := objectNamePrefix + "turtr"
	fileName := "../sample/BingWallpaper-2015-11-07.jpg"
	newFile := "upload-new-file-2.jpg"

	// Routines默认值，CP开启默认路径是fileName+.cp
	// 第一次上传，上传4片
	uploadPartHooker = ErrorHooker
	err := s.bucket.UploadFile(objectName, fileName, 100*1024, Checkpoint(true, ""))
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "ErrorHooker")
	uploadPartHooker = defaultUploadPart

	// check cp
	ucp := uploadCheckpoint{}
	err = ucp.load(fileName + ".cp")
	c.Assert(err, IsNil)
	c.Assert(ucp.Magic, Equals, uploadCpMagic)
	c.Assert(len(ucp.MD5), Equals, len("LC34jZU5xK4hlxi3Qn3XGQ=="))
	c.Assert(ucp.FilePath, Equals, fileName)
	c.Assert(ucp.FileStat.Size, Equals, int64(482048))
	c.Assert(len(ucp.FileStat.LastModified.String()) > 0, Equals, true)
	c.Assert(ucp.FileStat.MD5, Equals, "")
	c.Assert(ucp.ObjectKey, Equals, objectName)
	c.Assert(len(ucp.UploadID), Equals, len("3F79722737D1469980DACEDCA325BB52"))
	c.Assert(len(ucp.Parts), Equals, 5)
	c.Assert(len(ucp.todoParts()), Equals, 1)
	c.Assert(len(ucp.allParts()), Equals, 5)

	// 第二次上传，完成剩余的一片
	err = s.bucket.UploadFile(objectName, fileName, 100*1024, Checkpoint(true, ""))
	c.Assert(err, IsNil)

	os.Remove(newFile)
	err = s.bucket.GetObjectToFile(objectName, newFile)
	c.Assert(err, IsNil)

	eq, err := compareFiles(fileName, newFile)
	c.Assert(err, IsNil)
	c.Assert(eq, Equals, true)

	err = s.bucket.DeleteObject(objectName)
	c.Assert(err, IsNil)

	err = ucp.load(fileName + ".cp")
	c.Assert(err, NotNil)

	// Routines指定，CP指定
	uploadPartHooker = ErrorHooker
	err = s.bucket.UploadFile(objectName, fileName, 100*1024, Routines(2), Checkpoint(true, objectName+".cp"))
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "ErrorHooker")
	uploadPartHooker = defaultUploadPart

	// check cp
	ucp = uploadCheckpoint{}
	err = ucp.load(objectName + ".cp")
	c.Assert(err, IsNil)
	c.Assert(ucp.Magic, Equals, uploadCpMagic)
	c.Assert(len(ucp.MD5), Equals, len("LC34jZU5xK4hlxi3Qn3XGQ=="))
	c.Assert(ucp.FilePath, Equals, fileName)
	c.Assert(ucp.FileStat.Size, Equals, int64(482048))
	c.Assert(len(ucp.FileStat.LastModified.String()) > 0, Equals, true)
	c.Assert(ucp.FileStat.MD5, Equals, "")
	c.Assert(ucp.ObjectKey, Equals, objectName)
	c.Assert(len(ucp.UploadID), Equals, len("3F79722737D1469980DACEDCA325BB52"))
	c.Assert(len(ucp.Parts), Equals, 5)
	c.Assert(len(ucp.todoParts()), Equals, 1)
	c.Assert(len(ucp.allParts()), Equals, 5)

	err = s.bucket.UploadFile(objectName, fileName, 100*1024, Routines(3), Checkpoint(true, objectName+".cp"))
	c.Assert(err, IsNil)

	os.Remove(newFile)
	err = s.bucket.GetObjectToFile(objectName, newFile)
	c.Assert(err, IsNil)

	eq, err = compareFiles(fileName, newFile)
	c.Assert(err, IsNil)
	c.Assert(eq, Equals, true)

	err = s.bucket.DeleteObject(objectName)
	c.Assert(err, IsNil)

	err = ucp.load(objectName + ".cp")
	c.Assert(err, NotNil)

	// 一次完成上传，中间没有错误
	err = s.bucket.UploadFile(objectName, fileName, 100*1024, Routines(3), Checkpoint(true, ""))
	c.Assert(err, IsNil)

	os.Remove(newFile)
	err = s.bucket.GetObjectToFile(objectName, newFile)
	c.Assert(err, IsNil)

	eq, err = compareFiles(fileName, newFile)
	c.Assert(err, IsNil)
	c.Assert(eq, Equals, true)

	err = s.bucket.DeleteObject(objectName)
	c.Assert(err, IsNil)

	// 用多协程下载，中间没有错误
	err = s.bucket.UploadFile(objectName, fileName, 100*1024, Routines(10), Checkpoint(true, ""))
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
	err = s.bucket.UploadFile(objectName, fileName, 100*1024, Routines(3), Checkpoint(true, ""), Meta("myprop", "mypropval"))

	meta, err := s.bucket.GetObjectDetailedMeta(objectName)
	c.Assert(err, IsNil)
	c.Assert(meta.Get("X-Oss-Meta-Myprop"), Equals, "mypropval")

	os.Remove(newFile)
	err = s.bucket.GetObjectToFile(objectName, newFile)
	c.Assert(err, IsNil)

	eq, err = compareFiles(fileName, newFile)
	c.Assert(err, IsNil)
	c.Assert(eq, Equals, true)

	err = s.bucket.DeleteObject(objectName)
	c.Assert(err, IsNil)
}

// TestUploadRoutineWithoutRecovery 多线程无断点恢复的上传
func (s *OssUploadSuite) TestUploadRoutineWithRecoveryNegative(c *C) {
	objectName := objectNamePrefix + "turrn"
	fileName := "../sample/BingWallpaper-2015-11-07.jpg"

	// 本地文件不存在
	err := s.bucket.UploadFile(objectName, "NotExist", 100*1024, Checkpoint(true, ""))
	c.Assert(err, NotNil)

	err = s.bucket.UploadFile(objectName, "NotExist", 100*1024, Routines(2), Checkpoint(true, ""))
	c.Assert(err, NotNil)

	// 指定的分片大小无效
	err = s.bucket.UploadFile(objectName, fileName, 1024, Checkpoint(true, ""))
	c.Assert(err, NotNil)

	err = s.bucket.UploadFile(objectName, fileName, 1024, Routines(2), Checkpoint(true, ""))
	c.Assert(err, NotNil)

	err = s.bucket.UploadFile(objectName, fileName, 1024*1024*1024*100, Checkpoint(true, ""))
	c.Assert(err, NotNil)

	err = s.bucket.UploadFile(objectName, fileName, 1024*1024*1024*100, Routines(2), Checkpoint(true, ""))
	c.Assert(err, NotNil)
}

// TestUploadLocalFileChange 上传过程中文件修改了
func (s *OssUploadSuite) TestUploadLocalFileChange(c *C) {
	objectName := objectNamePrefix + "tulfc"
	fileName := "../sample/BingWallpaper-2015-11-07.jpg"
	localFile := "BingWallpaper-2015-11-07.jpg"
	newFile := "upload-new-file-3.jpg"

	os.Remove(localFile)
	err := copyFile(fileName, localFile)
	c.Assert(err, IsNil)

	// 第一次上传，上传4片
	uploadPartHooker = ErrorHooker
	err = s.bucket.UploadFile(objectName, localFile, 100*1024, Checkpoint(true, ""))
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "ErrorHooker")
	uploadPartHooker = defaultUploadPart

	os.Remove(localFile)
	err = copyFile(fileName, localFile)
	c.Assert(err, IsNil)

	// 文件修改，第二次上传全部分片重新上传
	err = s.bucket.UploadFile(objectName, localFile, 100*1024, Checkpoint(true, ""))
	c.Assert(err, IsNil)

	os.Remove(newFile)
	err = s.bucket.GetObjectToFile(objectName, newFile)
	c.Assert(err, IsNil)

	eq, err := compareFiles(fileName, newFile)
	c.Assert(err, IsNil)
	c.Assert(eq, Equals, true)

	err = s.bucket.DeleteObject(objectName)
	c.Assert(err, IsNil)
}

func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	return err
}
