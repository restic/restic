/*
 * Minio Go Library for Amazon S3 Compatible Cloud Storage (C) 2016 Minio, Inc.
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

// GetBucketNotification - get bucket notification at a given path.
func (c Client) GetBucketNotification(bucketName string) (bucketNotification BucketNotification, err error) {
	// Input validation.
	if err := isValidBucketName(bucketName); err != nil {
		return BucketNotification{}, err
	}
	notification, err := c.getBucketNotification(bucketName)
	if err != nil {
		return BucketNotification{}, err
	}
	return notification, nil
}

// Request server for notification rules.
func (c Client) getBucketNotification(bucketName string) (BucketNotification, error) {

	urlValues := make(url.Values)
	urlValues.Set("notification", "")

	// Execute GET on bucket to list objects.
	resp, err := c.executeMethod("GET", requestMetadata{
		bucketName:  bucketName,
		queryValues: urlValues,
	})

	defer closeResponse(resp)
	if err != nil {
		return BucketNotification{}, err
	}
	return processBucketNotificationResponse(bucketName, resp)

}

// processes the GetNotification http response from the server.
func processBucketNotificationResponse(bucketName string, resp *http.Response) (BucketNotification, error) {
	if resp.StatusCode != http.StatusOK {
		errResponse := httpRespToErrorResponse(resp, bucketName, "")
		return BucketNotification{}, errResponse
	}
	var bucketNotification BucketNotification
	err := xmlDecoder(resp.Body, &bucketNotification)
	if err != nil {
		return BucketNotification{}, err
	}
	return bucketNotification, nil
}
