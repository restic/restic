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
	"net/http"
	"strconv"
	"strings"
	"time"
)

// BucketExists verify if bucket exists and you have permission to access it.
func (c Client) BucketExists(bucketName string) error {
	if err := isValidBucketName(bucketName); err != nil {
		return err
	}
	req, err := c.newRequest("HEAD", requestMetadata{
		bucketName: bucketName,
	})
	if err != nil {
		return err
	}
	resp, err := c.httpClient.Do(req)
	defer closeResponse(resp)
	if err != nil {
		return err
	}
	if resp != nil {
		if resp.StatusCode != http.StatusOK {
			return HTTPRespToErrorResponse(resp, bucketName, "")
		}
	}
	return nil
}

// StatObject verifies if object exists and you have permission to access.
func (c Client) StatObject(bucketName, objectName string) (ObjectStat, error) {
	if err := isValidBucketName(bucketName); err != nil {
		return ObjectStat{}, err
	}
	if err := isValidObjectName(objectName); err != nil {
		return ObjectStat{}, err
	}
	// Instantiate a new request.
	req, err := c.newRequest("HEAD", requestMetadata{
		bucketName: bucketName,
		objectName: objectName,
	})
	if err != nil {
		return ObjectStat{}, err
	}
	resp, err := c.httpClient.Do(req)
	defer closeResponse(resp)
	if err != nil {
		return ObjectStat{}, err
	}
	if resp != nil {
		if resp.StatusCode != http.StatusOK {
			return ObjectStat{}, HTTPRespToErrorResponse(resp, bucketName, objectName)
		}
	}
	md5sum := strings.Trim(resp.Header.Get("ETag"), "\"") // trim off the odd double quotes
	size, err := strconv.ParseInt(resp.Header.Get("Content-Length"), 10, 64)
	if err != nil {
		return ObjectStat{}, ErrorResponse{
			Code:            "InternalError",
			Message:         "Content-Length is invalid. " + reportIssue,
			BucketName:      bucketName,
			Key:             objectName,
			RequestID:       resp.Header.Get("x-amz-request-id"),
			HostID:          resp.Header.Get("x-amz-id-2"),
			AmzBucketRegion: resp.Header.Get("x-amz-bucket-region"),
		}
	}
	date, err := time.Parse(http.TimeFormat, resp.Header.Get("Last-Modified"))
	if err != nil {
		return ObjectStat{}, ErrorResponse{
			Code:            "InternalError",
			Message:         "Last-Modified time format is invalid. " + reportIssue,
			BucketName:      bucketName,
			Key:             objectName,
			RequestID:       resp.Header.Get("x-amz-request-id"),
			HostID:          resp.Header.Get("x-amz-id-2"),
			AmzBucketRegion: resp.Header.Get("x-amz-bucket-region"),
		}
	}
	contentType := strings.TrimSpace(resp.Header.Get("Content-Type"))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	// Save object metadata info.
	var objectStat ObjectStat
	objectStat.ETag = md5sum
	objectStat.Key = objectName
	objectStat.Size = size
	objectStat.LastModified = date
	objectStat.ContentType = contentType
	return objectStat, nil
}
