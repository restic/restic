package sample

import (
	"fmt"
	"io/ioutil"
	"strings"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
)

// CnameSample 展示了Cname的用法
func CnameSample() {
	// NewClient
	client, err := oss.New(endpoint4Cname, accessID4Cname, accessKey4Cname,
		oss.UseCname(true))
	if err != nil {
		HandleError(err)
	}

	// CreateBucket
	err = client.CreateBucket(bucketName4Cname)
	if err != nil {
		HandleError(err)
	}

	// SetBucketACL
	err = client.SetBucketACL(bucketName4Cname, oss.ACLPrivate)
	if err != nil {
		HandleError(err)
	}

	// 查看Bucket ACL
	gbar, err := client.GetBucketACL(bucketName4Cname)
	if err != nil {
		HandleError(err)
	}
	fmt.Println("Bucket ACL:", gbar.ACL)

	// ListBuckets， cname用户不能使用该操作
	_, err = client.ListBuckets()
	if err == nil {
		HandleError(err)
	}

	bucket, err := client.Bucket(bucketName4Cname)
	if err != nil {
		HandleError(err)
	}

	objectValue := "长忆观潮，满郭人争江上望。来疑沧海尽成空，万面鼓声中。弄潮儿向涛头立，手把红旗旗不湿。别来几向梦中看，梦觉尚心寒。"

	// PutObject
	err = bucket.PutObject(objectKey, strings.NewReader(objectValue))
	if err != nil {
		HandleError(err)
	}

	// GetObject
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

	// PutObjectFromFile
	err = bucket.PutObjectFromFile(objectKey, localFile)
	if err != nil {
		HandleError(err)
	}

	// GetObjectToFile
	err = bucket.GetObjectToFile(objectKey, newPicName)
	if err != nil {
		HandleError(err)
	}

	// ListObjects
	lor, err := bucket.ListObjects()
	if err != nil {
		HandleError(err)
	}
	fmt.Println("objects:", lor.Objects)

	// DeleteObject
	err = bucket.DeleteObject(objectKey)
	if err != nil {
		HandleError(err)
	}

	fmt.Println("CnameSample completed")
}
