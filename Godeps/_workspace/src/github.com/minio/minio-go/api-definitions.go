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

package minio

import (
	"io"
	"time"
)

// BucketStat container for bucket metadata.
type BucketStat struct {
	// The name of the bucket.
	Name string
	// Date the bucket was created.
	CreationDate time.Time
}

// ObjectStat container for object metadata.
type ObjectStat struct {
	ETag         string
	Key          string
	LastModified time.Time
	Size         int64
	ContentType  string

	// Owner name.
	Owner struct {
		DisplayName string
		ID          string
	}

	// The class of storage used to store the object.
	StorageClass string

	// Error
	Err error
}

// ObjectMultipartStat container for multipart object metadata.
type ObjectMultipartStat struct {
	// Date and time at which the multipart upload was initiated.
	Initiated time.Time `type:"timestamp" timestampFormat:"iso8601"`

	Initiator initiator
	Owner     owner

	StorageClass string

	// Key of the object for which the multipart upload was initiated.
	Key  string
	Size int64

	// Upload ID that identifies the multipart upload.
	UploadID string `xml:"UploadId"`

	// Error
	Err error
}

// partData - container for each part.
type partData struct {
	MD5Sum     []byte
	Sha256Sum  []byte
	ReadCloser io.ReadCloser
	Size       int64
	Number     int // partData number.

	// Error
	Err error
}

// putObjectData - container for each single PUT operation.
type putObjectData struct {
	MD5Sum      []byte
	Sha256Sum   []byte
	ReadCloser  io.ReadCloser
	Size        int64
	ContentType string
}
