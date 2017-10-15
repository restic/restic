package sample

import (
	"fmt"
	"strings"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
)

// DeleteObjectSample 展示了删除单个文件、批量删除文件的方法
func DeleteObjectSample() {
	// 创建Bucket
	bucket, err := GetTestBucket(bucketName)
	if err != nil {
		HandleError(err)
	}

	var val = "抽刀断水水更流，举杯销愁愁更愁。 人生在世不称意，明朝散发弄扁舟。"

	// 场景1：删除Object
	err = bucket.PutObject(objectKey, strings.NewReader(val))
	if err != nil {
		HandleError(err)
	}

	err = bucket.DeleteObject(objectKey)
	if err != nil {
		HandleError(err)
	}

	// 场景2：删除多个Object
	err = bucket.PutObject(objectKey+"1", strings.NewReader(val))
	if err != nil {
		HandleError(err)
	}

	err = bucket.PutObject(objectKey+"2", strings.NewReader(val))
	if err != nil {
		HandleError(err)
	}

	delRes, err := bucket.DeleteObjects([]string{objectKey + "1", objectKey + "2"})
	if err != nil {
		HandleError(err)
	}
	fmt.Println("Del Res:", delRes)

	lsRes, err := bucket.ListObjects()
	if err != nil {
		HandleError(err)
	}
	fmt.Println("Objects:", getObjectsFormResponse(lsRes))

	// 场景3：删除多个Object，详细模式时返回的结果中会包含成功删除的Object，默认该模式
	err = bucket.PutObject(objectKey+"1", strings.NewReader(val))
	if err != nil {
		HandleError(err)
	}

	err = bucket.PutObject(objectKey+"2", strings.NewReader(val))
	if err != nil {
		HandleError(err)
	}

	delRes, err = bucket.DeleteObjects([]string{objectKey + "1", objectKey + "2"},
		oss.DeleteObjectsQuiet(false))
	if err != nil {
		HandleError(err)
	}
	fmt.Println("Detail Del Res:", delRes)

	lsRes, err = bucket.ListObjects()
	if err != nil {
		HandleError(err)
	}
	fmt.Println("Objects:", getObjectsFormResponse(lsRes))

	// 场景4：删除多个Object，简单模式返回的消息体中只包含删除出错的Object结果
	err = bucket.PutObject(objectKey+"1", strings.NewReader(val))
	if err != nil {
		HandleError(err)
	}

	err = bucket.PutObject(objectKey+"2", strings.NewReader(val))
	if err != nil {
		HandleError(err)
	}

	delRes, err = bucket.DeleteObjects([]string{objectKey + "1", objectKey + "2"}, oss.DeleteObjectsQuiet(true))
	if err != nil {
		HandleError(err)
	}
	fmt.Println("Sample Del Res:", delRes)

	lsRes, err = bucket.ListObjects()
	if err != nil {
		HandleError(err)
	}
	fmt.Println("Objects:", getObjectsFormResponse(lsRes))

	// 删除object和bucket
	err = DeleteTestBucketAndObject(bucketName)
	if err != nil {
		HandleError(err)
	}

	fmt.Println("DeleteObjectSample completed")
}
