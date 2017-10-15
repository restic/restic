package sample

import (
	"fmt"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
)

// BucketLoggingSample 展示了如何设置/读取/清除存储空间的日志(Bucket Logging)
func BucketLoggingSample() {
	// New Client
	client, err := oss.New(endpoint, accessID, accessKey)
	if err != nil {
		HandleError(err)
	}

	// 创建bucket
	err = client.CreateBucket(bucketName)
	if err != nil {
		HandleError(err)
	}
	// 创建Target bucket，存储访问日志
	var targetBucketName = "target-bucket"
	err = client.CreateBucket(targetBucketName)
	if err != nil {
		HandleError(err)
	}

	// 场景1：设置Logging，bucketName中以"prefix"为前缀的object的访问日志将被记录到targetBucketName
	err = client.SetBucketLogging(bucketName, targetBucketName, "prefix-1", true)
	if err != nil {
		HandleError(err)
	}

	// 场景2：设置Logging，bucketName中以"prefix"为前缀的object的访问日志将被记录到bucketName
	// 注意：相同bucket，相同prefix，多次设置后者会覆盖前者
	err = client.SetBucketLogging(bucketName, bucketName, "prefix-2", true)
	if err != nil {
		HandleError(err)
	}

	// 删除Bucket上的Logging设置
	err = client.DeleteBucketLogging(bucketName)
	if err != nil {
		HandleError(err)
	}

	// 场景3：设置但不生效
	err = client.SetBucketLogging(bucketName, targetBucketName, "prefix-3", false)
	if err != nil {
		HandleError(err)
	}

	// 获取Bucket上设置的Logging
	gbl, err := client.GetBucketLogging(bucketName)
	if err != nil {
		HandleError(err)
	}
	fmt.Println("Bucket Logging:", gbl.LoggingEnabled)

	err = client.SetBucketLogging(bucketName, bucketName, "prefix2", true)
	if err != nil {
		HandleError(err)
	}

	// 获取Bucket上设置的Logging
	gbl, err = client.GetBucketLogging(bucketName)
	if err != nil {
		HandleError(err)
	}
	fmt.Println("Bucket Logging:", gbl.LoggingEnabled)

	// 删除Bucket上的Logging设置
	err = client.DeleteBucketLogging(bucketName)
	if err != nil {
		HandleError(err)
	}

	// 删除bucket
	err = client.DeleteBucket(bucketName)
	if err != nil {
		HandleError(err)
	}
	err = client.DeleteBucket(targetBucketName)
	if err != nil {
		HandleError(err)
	}

	fmt.Println("BucketLoggingSample completed")
}
