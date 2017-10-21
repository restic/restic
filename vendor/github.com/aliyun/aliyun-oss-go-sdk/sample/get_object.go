package sample

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
)

// GetObjectSample 展示了流式下载、范围下载、断点续传下载的用法
func GetObjectSample() {
	// 创建Bucket
	bucket, err := GetTestBucket(bucketName)
	if err != nil {
		HandleError(err)
	}

	// 上传对象
	err = bucket.PutObjectFromFile(objectKey, localFile)
	if err != nil {
		HandleError(err)
	}

	// 场景1：下载object存储到ReadCloser，注意需要Close。
	body, err := bucket.GetObject(objectKey)
	if err != nil {
		HandleError(err)
	}
	data, err := ioutil.ReadAll(body)
	body.Close()
	if err != nil {
		HandleError(err)
	}
	data = data // use data

	// 场景2：下载object存储到bytes数组，适合小对象。
	buf := new(bytes.Buffer)
	body, err = bucket.GetObject(objectKey)
	if err != nil {
		HandleError(err)
	}
	io.Copy(buf, body)
	body.Close()

	// 场景3：下载object存储到本地文件，用户打开文件传入句柄。
	fd, err := os.OpenFile("mynewfile-1.jpg", os.O_WRONLY|os.O_CREATE, 0660)
	if err != nil {
		HandleError(err)
	}
	defer fd.Close()

	body, err = bucket.GetObject(objectKey)
	if err != nil {
		HandleError(err)
	}
	io.Copy(fd, body)
	body.Close()

	// 场景4：下载object存储到本地文件。
	err = bucket.GetObjectToFile(objectKey, "mynewfile-2.jpg")
	if err != nil {
		HandleError(err)
	}

	// 场景5：满足约束条件下载，否则返回错误。GetObject/GetObjectToFile/DownloadFile都支持该功能。
	// 修改时间，约束条件满足，执行下载。
	body, err = bucket.GetObject(objectKey, oss.IfModifiedSince(pastDate))
	if err != nil {
		HandleError(err)
	}
	body.Close()
	// 修改时间，约束条件不满足，不执行下载。
	_, err = bucket.GetObject(objectKey, oss.IfUnmodifiedSince(pastDate))
	if err == nil {
		HandleError(err)
	}

	meta, err := bucket.GetObjectDetailedMeta(objectKey)
	if err != nil {
		HandleError(err)
	}
	etag := meta.Get(oss.HTTPHeaderEtag)
	// 校验内容，约束条件满足，执行下载。
	body, err = bucket.GetObject(objectKey, oss.IfMatch(etag))
	if err != nil {
		HandleError(err)
	}
	body.Close()

	// 校验内容，约束条件不满足，不执行下载。
	body, err = bucket.GetObject(objectKey, oss.IfNoneMatch(etag))
	if err == nil {
		HandleError(err)
	}

	// 场景6：大文件分片下载，支持并发下载，断点续传功能。
	// 分片下载，分片大小为100K。默认使用不使用并发下载，不使用断点续传。
	err = bucket.DownloadFile(objectKey, "mynewfile-3.jpg", 100*1024)
	if err != nil {
		HandleError(err)
	}

	// 分片大小为100K，3个协程并发下载。
	err = bucket.DownloadFile(objectKey, "mynewfile-3.jpg", 100*1024, oss.Routines(3))
	if err != nil {
		HandleError(err)
	}

	// 分片大小为100K，3个协程并发下载，使用断点续传下载文件。
	err = bucket.DownloadFile(objectKey, "mynewfile-3.jpg", 100*1024, oss.Routines(3), oss.Checkpoint(true, ""))
	if err != nil {
		HandleError(err)
	}

	// 断点续传功能需要使用本地文件，记录哪些分片已经下载。该文件路径可以Checkpoint的第二个参数指定，如果为空，则为下载文件目录。
	err = bucket.DownloadFile(objectKey, "mynewfile-3.jpg", 100*1024, oss.Checkpoint(true, "mynewfile.cp"))
	if err != nil {
		HandleError(err)
	}

	// 场景7：内容进行 GZIP压缩传输的用户。GetObject/GetObjectToFile具有相同功能。
	err = bucket.PutObjectFromFile(objectKey, htmlLocalFile)
	if err != nil {
		HandleError(err)
	}

	err = bucket.GetObjectToFile(objectKey, "myhtml.gzip", oss.AcceptEncoding("gzip"))
	if err != nil {
		HandleError(err)
	}

	// 删除object和bucket
	err = DeleteTestBucketAndObject(bucketName)
	if err != nil {
		HandleError(err)
	}

	fmt.Println("GetObjectSample completed")
}
