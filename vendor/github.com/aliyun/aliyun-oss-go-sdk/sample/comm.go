package sample

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
)

var (
	pastDate   = time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC)
	futureDate = time.Date(2049, time.January, 10, 23, 0, 0, 0, time.UTC)
)

// HandleError sample中的错误处理
func HandleError(err error) {
	fmt.Println("occurred error:", err)
	os.Exit(-1)
}

// GetTestBucket 创建sample的Bucket并返回OssBucket对象，该函数为了简化sample，让sample代码更明了
func GetTestBucket(bucketName string) (*oss.Bucket, error) {
	// New Client
	client, err := oss.New(endpoint, accessID, accessKey)
	if err != nil {
		return nil, err
	}

	// Create Bucket
	err = client.CreateBucket(bucketName)
	if err != nil {
		return nil, err
	}

	// Get Bucket
	bucket, err := client.Bucket(bucketName)
	if err != nil {
		return nil, err
	}

	return bucket, nil
}

// DeleteTestBucketAndObject 删除sample的object和bucket，该函数为了简化sample，让sample代码更明了
func DeleteTestBucketAndObject(bucketName string) error {
	// New Client
	client, err := oss.New(endpoint, accessID, accessKey)
	if err != nil {
		return err
	}

	// Get Bucket
	bucket, err := client.Bucket(bucketName)
	if err != nil {
		return err
	}

	// Delete Part
	lmur, err := bucket.ListMultipartUploads()
	if err != nil {
		return err
	}

	for _, upload := range lmur.Uploads {
		var imur = oss.InitiateMultipartUploadResult{Bucket: bucket.BucketName,
			Key: upload.Key, UploadID: upload.UploadID}
		err = bucket.AbortMultipartUpload(imur)
		if err != nil {
			return err
		}
	}

	// Delete Objects
	lor, err := bucket.ListObjects()
	if err != nil {
		return err
	}

	for _, object := range lor.Objects {
		err = bucket.DeleteObject(object.Key)
		if err != nil {
			return err
		}
	}

	// Delete Bucket
	err = client.DeleteBucket(bucketName)
	if err != nil {
		return err
	}

	return nil
}

// Object pair of key and value
type Object struct {
	Key   string
	Value string
}

// CreateObjects 创建一组对象，该函数为了简化sample，让sample代码更明了
func CreateObjects(bucket *oss.Bucket, objects []Object) error {
	for _, object := range objects {
		err := bucket.PutObject(object.Key, strings.NewReader(object.Value))
		if err != nil {
			return err
		}
	}
	return nil
}

// DeleteObjects 删除sample的object和bucket，该函数为了简化sample，让sample代码更明了
func DeleteObjects(bucket *oss.Bucket, objects []Object) error {
	for _, object := range objects {
		err := bucket.DeleteObject(object.Key)
		if err != nil {
			return err
		}
	}
	return nil
}
