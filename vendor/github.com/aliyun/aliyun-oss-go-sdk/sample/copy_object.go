package sample

import (
	"fmt"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
)

// CopyObjectSample 展示了拷贝文件的用法
func CopyObjectSample() {
	// 创建Bucket
	bucket, err := GetTestBucket(bucketName)
	if err != nil {
		HandleError(err)
	}

	// 创建一个Object
	err = bucket.PutObjectFromFile(objectKey, localFile)
	if err != nil {
		HandleError(err)
	}

	// 场景1：把已经存在的对象copy成一个新对象
	var descObjectKey = "descobject"
	_, err = bucket.CopyObject(objectKey, descObjectKey)
	if err != nil {
		HandleError(err)
	}

	// 场景2：把已经存在的对象copy成一个新对象，目标对象存在时，会覆盖
	_, err = bucket.CopyObject(objectKey, descObjectKey)
	if err != nil {
		HandleError(err)
	}

	err = bucket.DeleteObject(descObjectKey)
	if err != nil {
		HandleError(err)
	}

	// 场景3：对象copy时对源对象执行约束条件，满足时候copy，不满足时返回错误，不执行copy
	// 约束条件不满足，copy没有执行
	_, err = bucket.CopyObject(objectKey, descObjectKey, oss.CopySourceIfModifiedSince(futureDate))
	if err == nil {
		HandleError(err)
	}
	fmt.Println("CopyObjectError:", err)
	// 约束条件满足，copy执行
	_, err = bucket.CopyObject(objectKey, descObjectKey, oss.CopySourceIfUnmodifiedSince(futureDate))
	if err != nil {
		HandleError(err)
	}

	// 场景4：对象copy时，可以指定目标对象的Properties，同时一定要指定MetadataDirective为MetaReplace
	options := []oss.Option{
		oss.Expires(futureDate),
		oss.Meta("myprop", "mypropval"),
		oss.MetadataDirective(oss.MetaReplace)}
	_, err = bucket.CopyObject(objectKey, descObjectKey, options...)
	if err != nil {
		HandleError(err)
	}

	meta, err := bucket.GetObjectDetailedMeta(descObjectKey)
	if err != nil {
		HandleError(err)
	}
	fmt.Println("meta:", meta)

	// 场景5：当源对象和目标对象相同时，目的是用来修改源对象的meta
	options = []oss.Option{
		oss.Expires(futureDate),
		oss.Meta("myprop", "mypropval"),
		oss.MetadataDirective(oss.MetaReplace)}

	_, err = bucket.CopyObject(objectKey, objectKey, options...)
	if err != nil {
		HandleError(err)
	}
	fmt.Println("meta:", meta)

	// 场景6：大文件分片拷贝，支持并发、断点续传功能。
	// 分片上传，分片大小为100K。默认使用不使用并发上传，不使用断点续传。
	err = bucket.CopyFile(bucketName, objectKey, descObjectKey, 100*1024)
	if err != nil {
		HandleError(err)
	}

	// 分片大小为100K，3个协程并发拷贝。
	err = bucket.CopyFile(bucketName, objectKey, descObjectKey, 100*1024, oss.Routines(3))
	if err != nil {
		HandleError(err)
	}

	// 分片大小为100K，3个协程并发拷贝，使用断点续传拷贝文件。
	err = bucket.CopyFile(bucketName, objectKey, descObjectKey, 100*1024, oss.Routines(3), oss.Checkpoint(true, ""))
	if err != nil {
		HandleError(err)
	}

	// 断点续传功能需要使用本地文件，记录哪些分片已经上传。该文件路径可以Checkpoint的第二个参数指定，如果为空，则为当前目录下的{descObjectKey}.cp。
	err = bucket.CopyFile(bucketName, objectKey, descObjectKey, 100*1024, oss.Checkpoint(true, localFile+".cp"))
	if err != nil {
		HandleError(err)
	}

	// 删除object和bucket
	err = DeleteTestBucketAndObject(bucketName)
	if err != nil {
		HandleError(err)
	}

	fmt.Println("CopyObjectSample completed")
}
