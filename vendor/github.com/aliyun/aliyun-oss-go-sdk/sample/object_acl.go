package sample

import (
	"fmt"
	"strings"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
)

// ObjectACLSample 展示了如何设置、读取文件权限(object acl)
func ObjectACLSample() {
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

	// 场景：设置Bucket ACL，可选权限有ACLPrivate、ACLPublicRead、ACLPublicReadWrite
	err = bucket.SetObjectACL(objectKey, oss.ACLPrivate)
	if err != nil {
		HandleError(err)
	}

	// 查看Object ACL，返回的权限标识为private、public-read、public-read-write其中之一
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

	fmt.Println("ObjectACLSample completed")
}
