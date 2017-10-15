package sample

import (
	"fmt"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
)

// ListObjectsSample 展示了列举文件的用法，包括默认参数列举、指定参数列举
func ListObjectsSample() {
	var myObjects = []Object{
		{"my-object-1", ""},
		{"my-object-11", ""},
		{"my-object-2", ""},
		{"my-object-21", ""},
		{"my-object-22", ""},
		{"my-object-3", ""},
		{"my-object-31", ""},
		{"my-object-32", ""}}

	// 创建Bucket
	bucket, err := GetTestBucket(bucketName)
	if err != nil {
		HandleError(err)
	}

	// 创建object
	err = CreateObjects(bucket, myObjects)
	if err != nil {
		HandleError(err)
	}

	// 场景1：使用默认参数参数
	lor, err := bucket.ListObjects()
	if err != nil {
		HandleError(err)
	}
	fmt.Println("my objects:", getObjectsFormResponse(lor))

	// 场景2：指定最大返回数量
	lor, err = bucket.ListObjects(oss.MaxKeys(3))
	if err != nil {
		HandleError(err)
	}
	fmt.Println("my objects max num:", getObjectsFormResponse(lor))

	// 场景3：返回指定前缀的Bucket
	lor, err = bucket.ListObjects(oss.Prefix("my-object-2"))
	if err != nil {
		HandleError(err)
	}
	fmt.Println("my objects prefix :", getObjectsFormResponse(lor))

	// 场景4：指定从某个之后返回
	lor, err = bucket.ListObjects(oss.Marker("my-object-22"))
	if err != nil {
		HandleError(err)
	}
	fmt.Println("my objects marker :", getObjectsFormResponse(lor))

	// 场景5：分页获取所有object，每次返回3个
	marker := oss.Marker("")
	for {
		lor, err = bucket.ListObjects(oss.MaxKeys(3), marker)
		if err != nil {
			HandleError(err)
		}
		marker = oss.Marker(lor.NextMarker)
		fmt.Println("my objects page :", getObjectsFormResponse(lor))
		if !lor.IsTruncated {
			break
		}
	}

	// 场景6：分页所有获取从某个之后的object，每次返回3个
	marker = oss.Marker("my-object-22")
	for {
		lor, err = bucket.ListObjects(oss.MaxKeys(3), marker)
		if err != nil {
			HandleError(err)
		}
		marker = oss.Marker(lor.NextMarker)
		fmt.Println("my objects marker&page :", getObjectsFormResponse(lor))
		if !lor.IsTruncated {
			break
		}
	}

	// 场景7：分页所有获取前缀的object，每次返回2个
	pre := oss.Prefix("my-object-2")
	marker = oss.Marker("")
	for {
		lor, err = bucket.ListObjects(oss.MaxKeys(2), marker, pre)
		if err != nil {
			HandleError(err)
		}
		pre = oss.Prefix(lor.Prefix)
		marker = oss.Marker(lor.NextMarker)
		fmt.Println("my objects prefix&page :", getObjectsFormResponse(lor))
		if !lor.IsTruncated {
			break
		}
	}

	err = DeleteObjects(bucket, myObjects)
	if err != nil {
		HandleError(err)
	}

	// 场景8：prefix和delimiter结合，完成分组功能，ListObjectsResponse.Objects表示不再组中，
	// ListObjectsResponse.CommonPrefixes分组结果
	myObjects = []Object{
		{"fun/test.txt", ""},
		{"fun/test.jpg", ""},
		{"fun/movie/001.avi", ""},
		{"fun/movie/007.avi", ""},
		{"fun/music/001.mp3", ""},
		{"fun/music/001.mp3", ""}}

	// 创建object
	err = CreateObjects(bucket, myObjects)
	if err != nil {
		HandleError(err)
	}

	lor, err = bucket.ListObjects(oss.Prefix("fun/"), oss.Delimiter("/"))
	if err != nil {
		HandleError(err)
	}
	fmt.Println("my objects prefix :", getObjectsFormResponse(lor),
		"common prefixes:", lor.CommonPrefixes)

	// 删除object和bucket
	err = DeleteTestBucketAndObject(bucketName)
	if err != nil {
		HandleError(err)
	}

	fmt.Println("ListObjectsSample completed")
}

func getObjectsFormResponse(lor oss.ListObjectsResult) string {
	var output string
	for _, object := range lor.Objects {
		output += object.Key + "  "
	}
	return output
}
