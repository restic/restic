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

package minio

import (
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"sort"
)

// GetBucketPolicy - get bucket policy at a given path.
func (c Client) GetBucketPolicy(bucketName, objectPrefix string) (bucketPolicy BucketPolicy, err error) {
	// Input validation.
	if err := isValidBucketName(bucketName); err != nil {
		return BucketPolicyNone, err
	}
	if err := isValidObjectPrefix(objectPrefix); err != nil {
		return BucketPolicyNone, err
	}
	policy, err := c.getBucketPolicy(bucketName, objectPrefix)
	if err != nil {
		return BucketPolicyNone, err
	}
	return identifyPolicyType(policy, bucketName, objectPrefix), nil
}

// Request server for policy.
func (c Client) getBucketPolicy(bucketName string, objectPrefix string) (BucketAccessPolicy, error) {
	// Get resources properly escaped and lined up before
	// using them in http request.
	urlValues := make(url.Values)
	urlValues.Set("policy", "")

	// Execute GET on bucket to list objects.
	resp, err := c.executeMethod("GET", requestMetadata{
		bucketName:  bucketName,
		queryValues: urlValues,
	})

	defer closeResponse(resp)
	if err != nil {
		return BucketAccessPolicy{}, err
	}
	return processBucketPolicyResponse(bucketName, resp)

}

// processes the GetPolicy http response from the server.
func processBucketPolicyResponse(bucketName string, resp *http.Response) (BucketAccessPolicy, error) {
	if resp != nil {
		if resp.StatusCode != http.StatusOK {
			errResponse := httpRespToErrorResponse(resp, bucketName, "")
			if ToErrorResponse(errResponse).Code == "NoSuchBucketPolicy" {
				return BucketAccessPolicy{Version: "2012-10-17"}, nil
			}
			return BucketAccessPolicy{}, errResponse
		}
	}
	// Read access policy up to maxAccessPolicySize.
	// http://docs.aws.amazon.com/AmazonS3/latest/dev/access-policy-language-overview.html
	// bucket policies are limited to 20KB in size, using a limit reader.
	bucketPolicyBuf, err := ioutil.ReadAll(io.LimitReader(resp.Body, maxAccessPolicySize))
	if err != nil {
		return BucketAccessPolicy{}, err
	}
	policy, err := unMarshalBucketPolicy(bucketPolicyBuf)
	if err != nil {
		return BucketAccessPolicy{}, err
	}
	// Sort the policy actions and resources for convenience.
	for _, statement := range policy.Statements {
		sort.Strings(statement.Actions)
		sort.Strings(statement.Resources)
	}
	return policy, nil
}
