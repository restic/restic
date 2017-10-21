package sample

import (
	"fmt"
	"strings"
	"time"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
)

// ArchiveSample Archive Sample
func ArchiveSample() {
	// create archive bucket
	client, err := oss.New(endpoint, accessID, accessKey)
	if err != nil {
		HandleError(err)
	}

	err = client.CreateBucket(bucketName, oss.StorageClass(oss.StorageArchive))
	if err != nil {
		HandleError(err)
	}

	archiveBucket, err := client.Bucket(bucketName)
	if err != nil {
		HandleError(err)
	}

	// put archive object
	var val = "花间一壶酒，独酌无相亲。 举杯邀明月，对影成三人。"
	err = archiveBucket.PutObject(objectKey, strings.NewReader(val))
	if err != nil {
		HandleError(err)
	}

	// check whether the object is archive class
	meta, err := archiveBucket.GetObjectDetailedMeta(objectKey)
	if err != nil {
		HandleError(err)
	}

	if meta.Get("X-Oss-Storage-Class") == string(oss.StorageArchive) {
		// restore object
		err = archiveBucket.RestoreObject(objectKey)
		if err != nil {
			HandleError(err)
		}

		// wait for restore completed
		meta, err = archiveBucket.GetObjectDetailedMeta(objectKey)
		for meta.Get("X-Oss-Restore") == "ongoing-request=\"true\"" {
			fmt.Println("x-oss-restore:" + meta.Get("X-Oss-Restore"))
			time.Sleep(1000 * time.Second)
			meta, err = archiveBucket.GetObjectDetailedMeta(objectKey)
		}
	}

	// get restored object
	err = archiveBucket.GetObjectToFile(objectKey, localFile)
	if err != nil {
		HandleError(err)
	}

	// restore repeatedly
	err = archiveBucket.RestoreObject(objectKey)

	// delete object and bucket
	err = DeleteTestBucketAndObject(bucketName)
	if err != nil {
		HandleError(err)
	}

	fmt.Println("ArchiveSample completed")
}
