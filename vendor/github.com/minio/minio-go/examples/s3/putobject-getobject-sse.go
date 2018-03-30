// +build ignore

/*
 * Minio Go Library for Amazon S3 Compatible Cloud Storage
 * Copyright 2017 Minio, Inc.
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
	"bytes"
	"io/ioutil"
	"log"

	"github.com/minio/minio-go/pkg/encrypt"

	minio "github.com/minio/minio-go"
)

func main() {
	// Note: YOUR-ACCESSKEYID, YOUR-SECRETACCESSKEY, my-testfile, my-bucketname and
	// my-objectname are dummy values, please replace them with original values.

	// New returns an Amazon S3 compatible client object. API compatibility (v2 or v4) is automatically
	// determined based on the Endpoint value.
	minioClient, err := minio.New("s3.amazonaws.com", "YOUR-ACCESSKEYID", "YOUR-SECRETACCESSKEY", true)
	if err != nil {
		log.Fatalln(err)
	}

	bucketName := "my-bucket"
	objectName := "my-encrypted-object"
	object := []byte("Hello again")

	encryption := encrypt.DefaultPBKDF([]byte("my secret password"), []byte(bucketName+objectName))
	_, err = minioClient.PutObject(bucketName, objectName, bytes.NewReader(object), int64(len(object)), minio.PutObjectOptions{
		ServerSideEncryption: encryption,
	})
	if err != nil {
		log.Fatalln(err)
	}

	reader, err := minioClient.GetObject(bucketName, objectName, minio.GetObjectOptions{ServerSideEncryption: encryption})
	if err != nil {
		log.Fatalln(err)
	}
	defer reader.Close()

	decBytes, err := ioutil.ReadAll(reader)
	if err != nil {
		log.Fatalln(err)
	}
	if !bytes.Equal(decBytes, object) {
		log.Fatalln("Expected %s, got %s", string(object), string(decBytes))
	}
}
