// main of samples

package main

import (
	"fmt"
    
	"github.com/aliyun/aliyun-oss-go-sdk/sample"
)

func main() {
	sample.CreateBucketSample()
	sample.NewBucketSample()
	sample.ListBucketsSample()
	sample.BucketACLSample()
	sample.BucketLifecycleSample()
	sample.BucketRefererSample()
	sample.BucketLoggingSample()
	sample.BucketCORSSample()

	sample.ObjectACLSample()
	sample.ObjectMetaSample()
	sample.ListObjectsSample()
	sample.DeleteObjectSample()
	sample.AppendObjectSample()
	sample.CopyObjectSample()
	sample.PutObjectSample()
	sample.GetObjectSample()

	sample.CnameSample()
	sample.SignURLSample()

	sample.ArchiveSample()

	fmt.Println("All samples completed")
}
