// Package sample examples
package sample

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
	"strings"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
)

// AppendObjectSample 展示了追加上传的用法
func AppendObjectSample() {
	// 创建Bucket
	bucket, err := GetTestBucket(bucketName)
	if err != nil {
		HandleError(err)
	}

	err = bucket.DeleteObject(objectKey)

	var str = "弃我去者，昨日之日不可留。 乱我心者，今日之日多烦忧！"
	var nextPos int64

	// 场景1：追加字符串到object
	// 第一次追加的位置是0，返回值为下一次追加的位置
	nextPos, err = bucket.AppendObject(objectKey, strings.NewReader(str), nextPos)
	if err != nil {
		HandleError(err)
	}

	// 第二次追加
	nextPos, err = bucket.AppendObject(objectKey, strings.NewReader(str), nextPos)
	if err != nil {
		HandleError(err)
	}

	// 下载
	body, err := bucket.GetObject(objectKey)
	if err != nil {
		HandleError(err)
	}
	data, err := ioutil.ReadAll(body)
	body.Close()
	if err != nil {
		HandleError(err)
	}
	fmt.Println(objectKey, ":", string(data))

	err = bucket.DeleteObject(objectKey)
	if err != nil {
		HandleError(err)
	}

	// 场景2：追加[]byte到object
	nextPos = 0
	// 第一次追加的位置是0，返回值为下一次追加的位置
	nextPos, err = bucket.AppendObject(objectKey, bytes.NewReader([]byte(str)), nextPos)
	if err != nil {
		HandleError(err)
	}

	// 第二次追加
	nextPos, err = bucket.AppendObject(objectKey, bytes.NewReader([]byte(str)), nextPos)
	if err != nil {
		HandleError(err)
	}

	// 下载
	body, err = bucket.GetObject(objectKey)
	if err != nil {
		HandleError(err)
	}
	data, err = ioutil.ReadAll(body)
	body.Close()
	if err != nil {
		HandleError(err)
	}
	fmt.Println(objectKey, ":", string(data))

	err = bucket.DeleteObject(objectKey)
	if err != nil {
		HandleError(err)
	}

	//场景3：本地文件追加到Object
	fd, err := os.Open(localFile)
	if err != nil {
		HandleError(err)
	}
	defer fd.Close()

	nextPos = 0
	nextPos, err = bucket.AppendObject(objectKey, fd, nextPos)
	if err != nil {
		HandleError(err)
	}

	// 场景4，您可以通过GetObjectDetailedMeta获取下次追加的位置
	props, err := bucket.GetObjectDetailedMeta(objectKey)
	nextPos, err = strconv.ParseInt(props.Get(oss.HTTPHeaderOssNextAppendPosition), 10, 0)
	if err != nil {
		HandleError(err)
	}

	nextPos, err = bucket.AppendObject(objectKey, strings.NewReader(str), nextPos)
	if err != nil {
		HandleError(err)
	}

	err = bucket.DeleteObject(objectKey)
	if err != nil {
		HandleError(err)
	}

	// 场景5：第一次追加操作时，可以指定Object的Properties，包括以"x-oss-meta-my"为前缀的用户自定义属性
	options := []oss.Option{
		oss.Expires(futureDate),
		oss.ObjectACL(oss.ACLPublicRead),
		oss.Meta("myprop", "mypropval")}
	nextPos = 0
	fd.Seek(0, os.SEEK_SET)
	nextPos, err = bucket.AppendObject(objectKey, strings.NewReader(str), nextPos, options...)
	if err != nil {
		HandleError(err)
	}
	// 第二次追加
	fd.Seek(0, os.SEEK_SET)
	nextPos, err = bucket.AppendObject(objectKey, strings.NewReader(str), nextPos)
	if err != nil {
		HandleError(err)
	}

	props, err = bucket.GetObjectDetailedMeta(objectKey)
	if err != nil {
		HandleError(err)
	}
	fmt.Println("myprop:", props.Get("x-oss-meta-myprop"))

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

	fmt.Println("AppendObjectSample completed")
}
