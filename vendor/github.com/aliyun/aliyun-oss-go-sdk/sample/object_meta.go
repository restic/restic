package sample

import (
	"fmt"
	"strings"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
)

// ObjectMetaSample 展示了如何设置、读取文件元数据(object meta)
func ObjectMetaSample() {
	// 创建Bucket
	bucket, err := GetTestBucket(bucketName)
	if err != nil {
		HandleError(err)
	}

	// 创建object
	err = bucket.PutObject(objectKey, strings.NewReader("YoursObjectValue"))
	if err != nil {
		HandleError(err)
	}

	// 场景：设置Bucket Meta，可以设置一个或多个属性。
	// 注意：Meta不区分大小写
	options := []oss.Option{
		oss.Expires(futureDate),
		oss.Meta("myprop", "mypropval")}
	err = bucket.SetObjectMeta(objectKey, options...)
	if err != nil {
		HandleError(err)
	}

	// 场景1：查看Object的meta，只返回少量基本meta信息，如ETag、Size、LastModified。
	props, err := bucket.GetObjectMeta(objectKey)
	if err != nil {
		HandleError(err)
	}
	fmt.Println("Object Meta:", props)

	// 场景2：查看Object的所有Meta，包括自定义的meta。
	props, err = bucket.GetObjectDetailedMeta(objectKey)
	if err != nil {
		HandleError(err)
	}
	fmt.Println("Expires:", props.Get("Expires"))

	// 场景3：查看Object的所有Meta，符合约束条件返回，不符合约束条件保存，包括自定义的meta。
	props, err = bucket.GetObjectDetailedMeta(objectKey, oss.IfUnmodifiedSince(futureDate))
	if err != nil {
		HandleError(err)
	}
	fmt.Println("MyProp:", props.Get("X-Oss-Meta-Myprop"))

	_, err = bucket.GetObjectDetailedMeta(objectKey, oss.IfModifiedSince(futureDate))
	if err == nil {
		HandleError(err)
	}

	goar, err := bucket.GetObjectACL(objectKey)
	if err != nil {
		HandleError(err)
	}
	fmt.Println("Object ACL:", goar.ACL)

	// 删除object和bucket
	err = DeleteTestBucketAndObject(bucketName)
	if err != nil {
		HandleError(err)
	}

	fmt.Println("ObjectMetaSample completed")
}
