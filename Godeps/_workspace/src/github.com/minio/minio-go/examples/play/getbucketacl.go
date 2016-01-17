// +build ignore

/*
 * Minio Go Library for Amazon S3 Compatible Cloud Storage (C) 2015 Minio, Inc.
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
	// Note: my-bucketname is a dummy value, please replace them with original value.

	// Requests are always secure by default. set inSecure=true to enable insecure access.
	// inSecure boolean is the last argument for New().

	// New provides a client object backend by automatically detected signature type based
	// on the provider.
	s3Client, err := minio.New("play.minio.io:9002", "Q3AM3UQ867SPQQA43P2F", "zuf+tfteSlswRu7BJ86wekitnifILbZam1KYY3TG", false)
	if err != nil {
		log.Fatalln(err)
	}

	acl, err := s3Client.GetBucketACL("my-bucketname")
	if err != nil {
		log.Fatalln(err)
	}
	log.Println(acl)

}
