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
	"net/url"
)

// RemoveBucket deletes the bucket name.
//
//  All objects (including all object versions and delete markers).
//  in the bucket must be deleted before successfully attempting this request.
func (c Client) RemoveBucket(bucketName string) error {
	if err := isValidBucketName(bucketName); err != nil {
		return err
	}
	req, err := c.newRequest("DELETE", requestMetadata{
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
		if resp.StatusCode != http.StatusNoContent {
			return HTTPRespToErrorResponse(resp, bucketName, "")
		}
	}

	// Remove the location from cache on a successful delete.
	c.bucketLocCache.Delete(bucketName)

	return nil
}

// RemoveObject remove an object from a bucket.
func (c Client) RemoveObject(bucketName, objectName string) error {
	if err := isValidBucketName(bucketName); err != nil {
		return err
	}
	if err := isValidObjectName(objectName); err != nil {
		return err
	}
	req, err := c.newRequest("DELETE", requestMetadata{
		bucketName: bucketName,
		objectName: objectName,
	})
	if err != nil {
		return err
	}
	resp, err := c.httpClient.Do(req)
	defer closeResponse(resp)
	if err != nil {
		return err
	}
	// DeleteObject always responds with http '204' even for
	// objects which do not exist. So no need to handle them
	// specifically.
	return nil
}

// RemoveIncompleteUpload aborts an partially uploaded object.
// Requires explicit authentication, no anonymous requests are allowed for multipart API.
func (c Client) RemoveIncompleteUpload(bucketName, objectName string) error {
	// Validate input arguments.
	if err := isValidBucketName(bucketName); err != nil {
		return err
	}
	if err := isValidObjectName(objectName); err != nil {
		return err
	}
	errorCh := make(chan error)
	go func(errorCh chan<- error) {
		defer close(errorCh)
		// Find multipart upload id of the object.
		uploadID, err := c.findUploadID(bucketName, objectName)
		if err != nil {
			errorCh <- err
			return
		}
		if uploadID != "" {
			// If uploadID is not an empty string, initiate the request.
			err := c.abortMultipartUpload(bucketName, objectName, uploadID)
			if err != nil {
				errorCh <- err
				return
			}
			return
		}
	}(errorCh)
	err, ok := <-errorCh
	if ok && err != nil {
		return err
	}
	return nil
}

// abortMultipartUpload aborts a multipart upload for the given uploadID, all parts are deleted.
func (c Client) abortMultipartUpload(bucketName, objectName, uploadID string) error {
	// Validate input arguments.
	if err := isValidBucketName(bucketName); err != nil {
		return err
	}
	if err := isValidObjectName(objectName); err != nil {
		return err
	}

	// Initialize url queries.
	urlValues := make(url.Values)
	urlValues.Set("uploadId", uploadID)

	// Instantiate a new DELETE request.
	req, err := c.newRequest("DELETE", requestMetadata{
		bucketName:  bucketName,
		objectName:  objectName,
		queryValues: urlValues,
	})
	if err != nil {
		return err
	}
	// execute the request.
	resp, err := c.httpClient.Do(req)
	defer closeResponse(resp)
	if err != nil {
		return err
	}
	if resp != nil {
		if resp.StatusCode != http.StatusNoContent {
			// Abort has no response body, handle it.
			var errorResponse ErrorResponse
			switch resp.StatusCode {
			case http.StatusNotFound:
				// This is needed specifically for Abort and it cannot be converged.
				errorResponse = ErrorResponse{
					Code:            "NoSuchUpload",
					Message:         "The specified multipart upload does not exist.",
					BucketName:      bucketName,
					Key:             objectName,
					RequestID:       resp.Header.Get("x-amz-request-id"),
					HostID:          resp.Header.Get("x-amz-id-2"),
					AmzBucketRegion: resp.Header.Get("x-amz-bucket-region"),
				}
			default:
				return HTTPRespToErrorResponse(resp, bucketName, objectName)
			}
			return errorResponse
		}
	}
	return nil
}
