package sample

import (
	"fmt"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
)

// BucketLifecycleSample 展示了如何设置/读取/清除存储空间中文件的生命周期(Bucket Lifecycle)
func BucketLifecycleSample() {
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

	// 场景1：设置Lifecycle，其中规则的id是id1，规则生效的object前缀是one，符合的Object绝对过期时间2015/11/11
	var rule1 = oss.BuildLifecycleRuleByDate("id1", "one", true, 2015, 11, 11)
	var rules = []oss.LifecycleRule{rule1}
	err = client.SetBucketLifecycle(bucketName, rules)
	if err != nil {
		HandleError(err)
	}

	// 场景2：设置Lifecycle，其中规则的id是id2，规则生效的object前缀是two，符合的Object相对过期时间是3天后
	var rule2 = oss.BuildLifecycleRuleByDays("id2", "two", true, 3)
	rules = []oss.LifecycleRule{rule2}
	err = client.SetBucketLifecycle(bucketName, rules)
	if err != nil {
		HandleError(err)
	}

	// 场景3：在Bucket上同时设置两条规格，两个规则分别作用与不同的对象。规则id相同是会覆盖老的规则。
	var rule3 = oss.BuildLifecycleRuleByDays("id1", "two", true, 365)
	var rule4 = oss.BuildLifecycleRuleByDate("id2", "one", true, 2016, 11, 11)
	rules = []oss.LifecycleRule{rule3, rule4}
	err = client.SetBucketLifecycle(bucketName, rules)
	if err != nil {
		HandleError(err)
	}

	// 获取Bucket上设置的Lifecycle
	gbl, err := client.GetBucketLifecycle(bucketName)
	if err != nil {
		HandleError(err)
	}
	fmt.Println("Bucket Lifecycle:", gbl.Rules)

	// 删除Bucket上的Lifecycle设置
	err = client.DeleteBucketLifecycle(bucketName)
	if err != nil {
		HandleError(err)
	}

	// 删除bucket
	err = client.DeleteBucket(bucketName)
	if err != nil {
		HandleError(err)
	}

	fmt.Println("BucketLifecycleSample completed")
}
