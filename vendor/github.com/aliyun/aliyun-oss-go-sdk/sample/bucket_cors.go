package sample

import (
	"fmt"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
)

// BucketCORSSample 展示了如何设置/读取/清除存储空间的跨域访问(Bucket CORS)
func BucketCORSSample() {
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

	rule1 := oss.CORSRule{
		AllowedOrigin: []string{"*"},
		AllowedMethod: []string{"PUT", "GET", "POST"},
		AllowedHeader: []string{},
		ExposeHeader:  []string{},
		MaxAgeSeconds: 100,
	}

	rule2 := oss.CORSRule{
		AllowedOrigin: []string{"http://www.a.com", "http://www.b.com"},
		AllowedMethod: []string{"GET"},
		AllowedHeader: []string{"Authorization"},
		ExposeHeader:  []string{"x-oss-test", "x-oss-test1"},
		MaxAgeSeconds: 100,
	}

	// 场景1：设置Bucket的CORS规则
	err = client.SetBucketCORS(bucketName, []oss.CORSRule{rule1})
	if err != nil {
		HandleError(err)
	}

	// 场景2：设置Bucket的CORS规则，如果该Bucket上已经设置了CORS规则，则会覆盖。
	err = client.SetBucketCORS(bucketName, []oss.CORSRule{rule1, rule2})
	if err != nil {
		HandleError(err)
	}

	// 获取Bucket上设置的CORS
	gbl, err := client.GetBucketCORS(bucketName)
	if err != nil {
		HandleError(err)
	}
	fmt.Println("Bucket CORS:", gbl.CORSRules)

	// 删除Bucket上的CORS设置
	err = client.DeleteBucketCORS(bucketName)
	if err != nil {
		HandleError(err)
	}

	// 删除bucket
	err = client.DeleteBucket(bucketName)
	if err != nil {
		HandleError(err)
	}

	fmt.Println("BucketCORSSample completed")
}
