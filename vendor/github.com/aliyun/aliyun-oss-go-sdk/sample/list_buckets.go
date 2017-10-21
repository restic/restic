package sample

import (
	"fmt"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
)

// ListBucketsSample 展示了列举存储空间的用法，包括默认参数列举、指定参数列举
func ListBucketsSample() {
	var myBuckets = []string{
		"my-bucket-1",
		"my-bucket-11",
		"my-bucket-2",
		"my-bucket-21",
		"my-bucket-22",
		"my-bucket-3",
		"my-bucket-31",
		"my-bucket-32"}

	// New Client
	client, err := oss.New(endpoint, accessID, accessKey)
	if err != nil {
		HandleError(err)
	}

	// remove other bucket
	lbr, err := client.ListBuckets()
	if err != nil {
		HandleError(err)
	}

	for _, bucket := range lbr.Buckets {
		err = client.DeleteBucket(bucket.Name)
		if err != nil {
			//HandleError(err)
		}
	}

	// 创建bucket
	for _, bucketName := range myBuckets {
		err = client.CreateBucket(bucketName)
		if err != nil {
			HandleError(err)
		}
	}

	// 场景1：使用默认参数参数
	lbr, err = client.ListBuckets()
	if err != nil {
		HandleError(err)
	}
	fmt.Println("my buckets:", lbr.Buckets)

	// 场景2：指定最大返回数量
	lbr, err = client.ListBuckets(oss.MaxKeys(3))
	if err != nil {
		HandleError(err)
	}
	fmt.Println("my buckets max num:", lbr.Buckets)

	// 场景3：返回指定前缀的Bucket
	lbr, err = client.ListBuckets(oss.Prefix("my-bucket-2"))
	if err != nil {
		HandleError(err)
	}
	fmt.Println("my buckets prefix :", lbr.Buckets)

	// 场景4：指定从某个之后返回
	lbr, err = client.ListBuckets(oss.Marker("my-bucket-22"))
	if err != nil {
		HandleError(err)
	}
	fmt.Println("my buckets marker :", lbr.Buckets)

	// 场景5：分页获取所有bucket，每次返回3个
	marker := oss.Marker("")
	for {
		lbr, err = client.ListBuckets(oss.MaxKeys(3), marker)
		if err != nil {
			HandleError(err)
		}
		marker = oss.Marker(lbr.NextMarker)
		fmt.Println("my buckets page :", lbr.Buckets)
		if !lbr.IsTruncated {
			break
		}
	}

	// 场景6：分页所有获取从某个之后的bucket，每次返回3个
	marker = oss.Marker("my-bucket-22")
	for {
		lbr, err = client.ListBuckets(oss.MaxKeys(3), marker)
		if err != nil {
			HandleError(err)
		}
		marker = oss.Marker(lbr.NextMarker)
		fmt.Println("my buckets marker&page :", lbr.Buckets)
		if !lbr.IsTruncated {
			break
		}
	}

	// 场景7：分页所有获取前缀的bucket，每次返回3个
	pre := oss.Prefix("my-bucket-2")
	marker = oss.Marker("")
	for {
		lbr, err = client.ListBuckets(oss.MaxKeys(3), pre, marker)
		if err != nil {
			HandleError(err)
		}
		pre = oss.Prefix(lbr.Prefix)
		marker = oss.Marker(lbr.NextMarker)
		fmt.Println("my buckets prefix&page :", lbr.Buckets)
		if !lbr.IsTruncated {
			break
		}
	}

	// 删除bucket
	for _, bucketName := range myBuckets {
		err = client.DeleteBucket(bucketName)
		if err != nil {
			HandleError(err)
		}
	}

	fmt.Println("ListsBucketSample completed")
}
