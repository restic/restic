package sample

import (
	"fmt"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
)

// CreateBucketSample 展示了如何创建存储空间
func CreateBucketSample() {
	// New Client
	client, err := oss.New(endpoint, accessID, accessKey)
	if err != nil {
		HandleError(err)
	}

	DeleteTestBucketAndObject(bucketName)

	// 场景1：使用默认参数创建bucket
	err = client.CreateBucket(bucketName)
	if err != nil {
		HandleError(err)
	}

	// 删除bucket
	err = client.DeleteBucket(bucketName)
	if err != nil {
		HandleError(err)
	}

	// 场景2：创建bucket时指定其权限
	err = client.CreateBucket(bucketName, oss.ACL(oss.ACLPublicRead))
	if err != nil {
		HandleError(err)
	}

	// 场景3：重复创建OSS不会报错，但是不做任何操作，指定的ACL无效
	err = client.CreateBucket(bucketName, oss.ACL(oss.ACLPublicReadWrite))
	if err != nil {
		HandleError(err)
	}

	// 删除bucket
	err = client.DeleteBucket(bucketName)
	if err != nil {
		HandleError(err)
	}

	fmt.Println("CreateBucketSample completed")
}
