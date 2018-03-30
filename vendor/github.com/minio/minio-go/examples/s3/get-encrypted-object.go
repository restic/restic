// +build ignore

/*
 * Minio Go Library for Amazon S3 Compatible Cloud Storage
 * Copyright 2015-2017 Minio, Inc.
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
	"io"
	"log"
	"os"

	"github.com/minio/minio-go"
	"github.com/minio/minio-go/pkg/encrypt"
)

func main() {
	// Note: YOUR-ACCESSKEYID, YOUR-SECRETACCESSKEY, my-bucketname, my-objectname and
	// my-testfile are dummy values, please replace them with original values.

	// Requests are always secure (HTTPS) by default. Set secure=false to enable insecure (HTTP) access.
	// This boolean value is the last argument for New().

	// New returns an Amazon S3 compatible client object. API compatibility (v2 or v4) is automatically
	// determined based on the Endpoint value.
	s3Client, err := minio.New("s3.amazonaws.com", "YOUR-ACCESS-KEY-HERE", "YOUR-SECRET-KEY-HERE", true)
	if err != nil {
		log.Fatalln(err)
	}

	bucketname := "my-bucketname"              // Specify a bucket name - the bucket must already exist
	objectName := "my-objectname"              // Specify a object name - the object must already exist
	password := "correct horse battery staple" // Specify your password. DO NOT USE THIS ONE - USE YOUR OWN.

	// New SSE-C where the cryptographic key is derived from a password and the objectname + bucketname as salt
	encryption := encrypt.DefaultPBKDF([]byte(password), []byte(bucketname+objectName))

	// Get the encrypted object
	reader, err := s3Client.GetObject(bucketname, objectName, minio.GetObjectOptions{ServerSideEncryption: encryption})
	if err != nil {
		log.Fatalln(err)
	}
	defer reader.Close()

	// Local file which holds plain data
	localFile, err := os.Create("my-testfile")
	if err != nil {
		log.Fatalln(err)
	}
	defer localFile.Close()

	if _, err := io.Copy(localFile, reader); err != nil {
		log.Fatalln(err)
	}
}
