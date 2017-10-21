package sample

import (
	"bytes"
	"fmt"
	"os"
	"strings"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
)

// PutObjectSample 展示了简单上传、断点续传的使用方法
func PutObjectSample() {
	// 创建Bucket
	bucket, err := GetTestBucket(bucketName)
	if err != nil {
		HandleError(err)
	}

	var val = "花间一壶酒，独酌无相亲。 举杯邀明月，对影成三人。"

	// 场景1：上传object，value是字符串。
	err = bucket.PutObject(objectKey, strings.NewReader(val))
	if err != nil {
		HandleError(err)
	}

	// 场景2：上传object，value是[]byte。
	err = bucket.PutObject(objectKey, bytes.NewReader([]byte(val)))
	if err != nil {
		HandleError(err)
	}

	// 场景3：上传本地文件，用户打开文件，传入句柄。
	fd, err := os.Open(localFile)
	if err != nil {
		HandleError(err)
	}
	defer fd.Close()

	err = bucket.PutObject(objectKey, fd)
	if err != nil {
		HandleError(err)
	}

	// 场景4：上传本地文件，不需要打开文件。
	err = bucket.PutObjectFromFile(objectKey, localFile)
	if err != nil {
		HandleError(err)
	}

	// 场景5：上传object，上传时指定对象属性。PutObject/PutObjectFromFile/UploadFile都支持该功能。
	options := []oss.Option{
		oss.Expires(futureDate),
		oss.ObjectACL(oss.ACLPublicRead),
		oss.Meta("myprop", "mypropval"),
	}
	err = bucket.PutObject(objectKey, strings.NewReader(val), options...)
	if err != nil {
		HandleError(err)
	}

	props, err := bucket.GetObjectDetailedMeta(objectKey)
	if err != nil {
		HandleError(err)
	}
	fmt.Println("Object Meta:", props)

	// 场景6：大文件分片上传，支持并发上传，断点续传功能。
	// 分片上传，分片大小为100K。默认使用不使用并发上传，不使用断点续传。
	err = bucket.UploadFile(objectKey, localFile, 100*1024)
	if err != nil {
		HandleError(err)
	}

	// 分片大小为100K，3个协程并发上传。
	err = bucket.UploadFile(objectKey, localFile, 100*1024, oss.Routines(3))
	if err != nil {
		HandleError(err)
	}

	// 分片大小为100K，3个协程并发下载，使用断点续传上传文件。
	err = bucket.UploadFile(objectKey, localFile, 100*1024, oss.Routines(3), oss.Checkpoint(true, ""))
	if err != nil {
		HandleError(err)
	}

	// 断点续传功能需要使用本地文件，记录哪些分片已经上传。该文件路径可以Checkpoint的第二个参数指定，如果为空，则为上传文件目录。
	err = bucket.UploadFile(objectKey, localFile, 100*1024, oss.Checkpoint(true, localFile+".cp"))
	if err != nil {
		HandleError(err)
	}

	// 删除object和bucket
	err = DeleteTestBucketAndObject(bucketName)
	if err != nil {
		HandleError(err)
	}

	fmt.Println("PutObjectSample completed")
}
