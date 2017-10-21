package sample

import (
	"fmt"
	"strings"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
)

// NewBucketSample 展示了如何初始化Client、Bucket
func NewBucketSample() {
	// New Client
	client, err := oss.New(endpoint, accessID, accessKey)
	if err != nil {
		HandleError(err)
	}

	// Create Bucket
	err = client.CreateBucket(bucketName)
	if err != nil {
		HandleError(err)
	}

	// New Bucket
	bucket, err := client.Bucket(bucketName)
	if err != nil {
		HandleError(err)
	}

	// Put Object，上传一个Object
	var objectName = "myobject"
	err = bucket.PutObject(objectName, strings.NewReader("MyObjectValue"))
	if err != nil {
		HandleError(err)
	}

	// Delete Object，删除Object
	err = bucket.DeleteObject(objectName)
	if err != nil {
		HandleError(err)
	}

	// 删除bucket
	err = client.DeleteBucket(bucketName)
	if err != nil {
		HandleError(err)
	}

	fmt.Println("NewBucketSample completed")
}
