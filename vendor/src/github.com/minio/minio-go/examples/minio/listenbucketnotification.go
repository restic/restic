// +build ignore

/*
 * Minio Go Library for Amazon S3 Compatible Cloud Storage (C) 2015, 2016 Minio, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package main

import (
	"log"

	"github.com/minio/minio-go"
)

func main() {
	// Note: YOUR-ACCESSKEYID, YOUR-SECRETACCESSKEY and my-bucketname are
	// dummy values, please replace them with original values.

	// Requests are always secure (HTTPS) by default. Set secure=false to enable insecure (HTTP) access.
	// This boolean value is the last argument for New().

	// New returns an Amazon S3 compatible client object. API compatibility (v2 or v4) is automatically
	// determined based on the Endpoint value.
	minioClient, err := minio.New("play.minio.io:9000", "YOUR-ACCESSKEYID", "YOUR-SECRETACCESSKEY", true)
	if err != nil {
		log.Fatalln(err)
	}

	// s3Client.TraceOn(os.Stderr)

	// Create a done channel to control 'ListenBucketNotification' go routine.
	doneCh := make(chan struct{})

	// Indicate to our routine to exit cleanly upon return.
	defer close(doneCh)

	// Fetch the bucket location.
	location, err := minioClient.GetBucketLocation("YOUR-BUCKET")
	if err != nil {
		log.Fatalln(err)
	}

	// Construct a new account Arn.
	accountArn := minio.NewArn("minio", "sns", location, "your-account-id", "listen")
	topicConfig := minio.NewNotificationConfig(accountArn)
	topicConfig.AddEvents(minio.ObjectCreatedAll, minio.ObjectRemovedAll)
	topicConfig.AddFilterPrefix("photos/")
	topicConfig.AddFilterSuffix(".jpg")

	// Now, set all previously created notification configs
	bucketNotification := minio.BucketNotification{}
	bucketNotification.AddTopic(topicConfig)
	err = minioClient.SetBucketNotification("YOUR-BUCKET", bucketNotification)
	if err != nil {
		log.Fatalln("Error: " + err.Error())
	}
	log.Println("Success")

	// Listen for bucket notifications on "mybucket" filtered by accountArn "arn:minio:sns:<location>:<your-account-id>:listen".
	for notificationInfo := range minioClient.ListenBucketNotification("YOUR-BUCKET", accountArn, doneCh) {
		if notificationInfo.Err != nil {
			log.Fatalln(notificationInfo.Err)
		}
		log.Println(notificationInfo)
	}
}
