package sample

import (
	"fmt"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
)

// BucketRefererSample 展示了如何设置/读取/清除存储空间的白名单(Bucket Referer)
func BucketRefererSample() {
	// New Client
	client, err := oss.New(endpoint, accessID, accessKey)
	if err != nil {
		HandleError(err)
	}

	// 使用默认参数创建bucket
	err = client.CreateBucket(bucketName)
	if err != nil {
		HandleError(err)
	}

	var referers = []string{
		"http://www.aliyun.com",
		"http://www.???.aliyuncs.com",
		"http://www.*.com",
	}

	// 场景1：设置referers，referer中支持?和*，分布代替一个或多个字符
	err = client.SetBucketReferer(bucketName, referers, false)
	if err != nil {
		HandleError(err)
	}

	// 场景2：清空referers
	referers = []string{}
	err = client.SetBucketReferer(bucketName, referers, true)
	if err != nil {
		HandleError(err)
	}

	// 获取Bucket上设置的Lifecycle
	gbr, err := client.GetBucketReferer(bucketName)
	if err != nil {
		HandleError(err)
	}
	fmt.Println("Bucket Referers:", gbr.RefererList,
		"AllowEmptyReferer:", gbr.AllowEmptyReferer)

	// 删除bucket
	err = client.DeleteBucket(bucketName)
	if err != nil {
		HandleError(err)
	}

	fmt.Println("BucketRefererSample completed")
}
