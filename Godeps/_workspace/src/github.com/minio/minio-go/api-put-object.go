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
	"bytes"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
)

// getReaderSize gets the size of the underlying reader, if possible.
func getReaderSize(reader io.Reader) (size int64, err error) {
	size = -1
	if reader != nil {
		switch v := reader.(type) {
		case *bytes.Buffer:
			size = int64(v.Len())
		case *bytes.Reader:
			size = int64(v.Len())
		case *strings.Reader:
			size = int64(v.Len())
		case *os.File:
			var st os.FileInfo
			st, err = v.Stat()
			if err != nil {
				return 0, err
			}
			size = st.Size()
		case *Object:
			var st ObjectInfo
			st, err = v.Stat()
			if err != nil {
				return 0, err
			}
			size = st.Size
		}
	}
	return size, nil
}

// completedParts is a collection of parts sortable by their part numbers.
// used for sorting the uploaded parts before completing the multipart request.
type completedParts []completePart

func (a completedParts) Len() int           { return len(a) }
func (a completedParts) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a completedParts) Less(i, j int) bool { return a[i].PartNumber < a[j].PartNumber }

// PutObject creates an object in a bucket.
//
// You must have WRITE permissions on a bucket to create an object.
//
//  - For size smaller than 5MiB PutObject automatically does a single atomic Put operation.
//  - For size larger than 5MiB PutObject automatically does a resumable multipart Put operation.
//  - For size input as -1 PutObject does a multipart Put operation until input stream reaches EOF.
//    Maximum object size that can be uploaded through this operation will be 5TiB.
//
// NOTE: Google Cloud Storage does not implement Amazon S3 Compatible multipart PUT.
// So we fall back to single PUT operation with the maximum limit of 5GiB.
//
// NOTE: For anonymous requests Amazon S3 doesn't allow multipart upload. So we fall back to single PUT operation.
func (c Client) PutObject(bucketName, objectName string, reader io.Reader, contentType string) (n int64, err error) {
	// Input validation.
	if err := isValidBucketName(bucketName); err != nil {
		return 0, err
	}
	if err := isValidObjectName(objectName); err != nil {
		return 0, err
	}

	// get reader size.
	size, err := getReaderSize(reader)
	if err != nil {
		return 0, err
	}

	// Check for largest object size allowed.
	if size > int64(maxMultipartPutObjectSize) {
		return 0, ErrEntityTooLarge(size, bucketName, objectName)
	}

	// NOTE: Google Cloud Storage does not implement Amazon S3 Compatible multipart PUT.
	// So we fall back to single PUT operation with the maximum limit of 5GiB.
	if isGoogleEndpoint(c.endpointURL) {
		if size <= -1 {
			return 0, ErrorResponse{
				Code:       "NotImplemented",
				Message:    "Content-Length cannot be negative for file uploads to Google Cloud Storage.",
				Key:        objectName,
				BucketName: bucketName,
			}
		}
		if size > maxSinglePutObjectSize {
			return 0, ErrEntityTooLarge(size, bucketName, objectName)
		}
		// Do not compute MD5 for Google Cloud Storage. Uploads upto 5GiB in size.
		return c.putObjectNoChecksum(bucketName, objectName, reader, size, contentType)
	}

	// NOTE: S3 doesn't allow anonymous multipart requests.
	if isAmazonEndpoint(c.endpointURL) && c.anonymous {
		if size <= -1 {
			return 0, ErrorResponse{
				Code:       "NotImplemented",
				Message:    "Content-Length cannot be negative for anonymous requests.",
				Key:        objectName,
				BucketName: bucketName,
			}
		}
		if size > maxSinglePutObjectSize {
			return 0, ErrEntityTooLarge(size, bucketName, objectName)
		}
		// Do not compute MD5 for anonymous requests to Amazon S3. Uploads upto 5GiB in size.
		return c.putObjectNoChecksum(bucketName, objectName, reader, size, contentType)
	}

	// putSmall object.
	if size < minimumPartSize && size > 0 {
		return c.putObjectSingle(bucketName, objectName, reader, size, contentType)
	}
	// For all sizes greater than 5MiB do multipart.
	n, err = c.putObjectMultipart(bucketName, objectName, reader, size, contentType)
	if err != nil {
		errResp := ToErrorResponse(err)
		// Verify if multipart functionality is not available, if not
		// fall back to single PutObject operation.
		if errResp.Code == "NotImplemented" {
			// Verify if size of reader is greater than '5GiB'.
			if size > maxSinglePutObjectSize {
				return 0, ErrEntityTooLarge(size, bucketName, objectName)
			}
			// Fall back to uploading as single PutObject operation.
			return c.putObjectSingle(bucketName, objectName, reader, size, contentType)
		}
		return n, err
	}
	return n, nil
}

// putObjectNoChecksum special function used Google Cloud Storage. This special function
// is used for Google Cloud Storage since Google's multipart API is not S3 compatible.
func (c Client) putObjectNoChecksum(bucketName, objectName string, reader io.Reader, size int64, contentType string) (n int64, err error) {
	// Input validation.
	if err := isValidBucketName(bucketName); err != nil {
		return 0, err
	}
	if err := isValidObjectName(objectName); err != nil {
		return 0, err
	}
	if size > maxSinglePutObjectSize {
		return 0, ErrEntityTooLarge(size, bucketName, objectName)
	}
	// This function does not calculate sha256 and md5sum for payload.
	// Execute put object.
	st, err := c.putObjectDo(bucketName, objectName, ioutil.NopCloser(reader), nil, nil, size, contentType)
	if err != nil {
		return 0, err
	}
	if st.Size != size {
		return 0, ErrUnexpectedEOF(st.Size, size, bucketName, objectName)
	}
	return size, nil
}

// putObjectSingle is a special function for uploading single put object request.
// This special function is used as a fallback when multipart upload fails.
func (c Client) putObjectSingle(bucketName, objectName string, reader io.Reader, size int64, contentType string) (n int64, err error) {
	// Input validation.
	if err := isValidBucketName(bucketName); err != nil {
		return 0, err
	}
	if err := isValidObjectName(objectName); err != nil {
		return 0, err
	}
	if size > maxSinglePutObjectSize {
		return 0, ErrEntityTooLarge(size, bucketName, objectName)
	}
	// If size is a stream, upload upto 5GiB.
	if size <= -1 {
		size = maxSinglePutObjectSize
	}
	// Initialize a new temporary file.
	tmpFile, err := newTempFile("single$-putobject-single")
	if err != nil {
		return 0, err
	}
	md5Sum, sha256Sum, size, err := c.hashCopyN(tmpFile, reader, size)
	if err != nil {
		if err != io.EOF {
			return 0, err
		}
	}
	// Execute put object.
	st, err := c.putObjectDo(bucketName, objectName, tmpFile, md5Sum, sha256Sum, size, contentType)
	if err != nil {
		return 0, err
	}
	if st.Size != size {
		return 0, ErrUnexpectedEOF(st.Size, size, bucketName, objectName)
	}
	return size, nil
}

// putObjectDo - executes the put object http operation.
// NOTE: You must have WRITE permissions on a bucket to add an object to it.
func (c Client) putObjectDo(bucketName, objectName string, reader io.ReadCloser, md5Sum []byte, sha256Sum []byte, size int64, contentType string) (ObjectInfo, error) {
	// Input validation.
	if err := isValidBucketName(bucketName); err != nil {
		return ObjectInfo{}, err
	}
	if err := isValidObjectName(objectName); err != nil {
		return ObjectInfo{}, err
	}

	if size <= -1 {
		return ObjectInfo{}, ErrEntityTooSmall(size, bucketName, objectName)
	}

	if size > maxSinglePutObjectSize {
		return ObjectInfo{}, ErrEntityTooLarge(size, bucketName, objectName)
	}

	if strings.TrimSpace(contentType) == "" {
		contentType = "application/octet-stream"
	}

	// Set headers.
	customHeader := make(http.Header)
	customHeader.Set("Content-Type", contentType)

	// Populate request metadata.
	reqMetadata := requestMetadata{
		bucketName:         bucketName,
		objectName:         objectName,
		customHeader:       customHeader,
		contentBody:        reader,
		contentLength:      size,
		contentMD5Bytes:    md5Sum,
		contentSHA256Bytes: sha256Sum,
	}
	// Initiate new request.
	req, err := c.newRequest("PUT", reqMetadata)
	if err != nil {
		return ObjectInfo{}, err
	}
	// Execute the request.
	resp, err := c.do(req)
	defer closeResponse(resp)
	if err != nil {
		return ObjectInfo{}, err
	}
	if resp != nil {
		if resp.StatusCode != http.StatusOK {
			return ObjectInfo{}, HTTPRespToErrorResponse(resp, bucketName, objectName)
		}
	}

	var metadata ObjectInfo
	// Trim off the odd double quotes from ETag in the beginning and end.
	metadata.ETag = strings.TrimPrefix(resp.Header.Get("ETag"), "\"")
	metadata.ETag = strings.TrimSuffix(metadata.ETag, "\"")
	// A success here means data was written to server successfully.
	metadata.Size = size

	// Return here.
	return metadata, nil
}
