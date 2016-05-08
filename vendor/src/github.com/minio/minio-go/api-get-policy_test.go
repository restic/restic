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
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"reflect"
	"testing"
)

// Mocks valid http response containing bucket policy from server.
func generatePolicyResponse(resp *http.Response, policy BucketAccessPolicy) (*http.Response, error) {
	policyBytes, err := json.Marshal(policy)
	if err != nil {
		return nil, err
	}
	resp.StatusCode = http.StatusOK
	resp.Body = ioutil.NopCloser(bytes.NewBuffer(policyBytes))
	return resp, nil
}

// Tests the processing of GetPolicy response from server.
func TestProcessBucketPolicyResopnse(t *testing.T) {
	bucketAccesPolicies := []BucketAccessPolicy{
		{Version: "1.0"},
		{Version: "1.0", Statements: setReadOnlyStatement("minio-bucket", "")},
		{Version: "1.0", Statements: setReadWriteStatement("minio-bucket", "Asia/")},
		{Version: "1.0", Statements: setWriteOnlyStatement("minio-bucket", "Asia/India/")},
	}

	APIErrors := []APIError{
		{
			Code:           "NoSuchBucketPolicy",
			Description:    "The specified bucket does not have a bucket policy.",
			HTTPStatusCode: http.StatusNotFound,
		},
	}
	testCases := []struct {
		bucketName string
		isAPIError bool
		apiErr     APIError
		// expected results.
		expectedResult BucketAccessPolicy
		err            error
		// flag indicating whether tests should pass.
		shouldPass bool
	}{
		{"my-bucket", true, APIErrors[0], BucketAccessPolicy{Version: "2012-10-17"}, nil, true},
		{"my-bucket", false, APIError{}, bucketAccesPolicies[0], nil, true},
		{"my-bucket", false, APIError{}, bucketAccesPolicies[1], nil, true},
		{"my-bucket", false, APIError{}, bucketAccesPolicies[2], nil, true},
		{"my-bucket", false, APIError{}, bucketAccesPolicies[3], nil, true},
	}

	for i, testCase := range testCases {
		inputResponse := &http.Response{}
		var err error
		if testCase.isAPIError {
			inputResponse = generateErrorResponse(inputResponse, testCase.apiErr, testCase.bucketName)
		} else {
			inputResponse, err = generatePolicyResponse(inputResponse, testCase.expectedResult)
			if err != nil {
				t.Fatalf("Test %d: Creation of valid response failed", i+1)
			}
		}
		actualResult, err := processBucketPolicyResponse("my-bucket", inputResponse)
		if err != nil && testCase.shouldPass {
			t.Errorf("Test %d: Expected to pass, but failed with: <ERROR> %s", i+1, err.Error())
		}
		if err == nil && !testCase.shouldPass {
			t.Errorf("Test %d: Expected to fail with <ERROR> \"%s\", but passed instead", i+1, testCase.err.Error())
		}
		// Failed as expected, but does it fail for the expected reason.
		if err != nil && !testCase.shouldPass {
			if err.Error() != testCase.err.Error() {
				t.Errorf("Test %d: Expected to fail with error \"%s\", but instead failed with error \"%s\" instead", i+1, testCase.err.Error(), err.Error())
			}
		}
		if err == nil && testCase.shouldPass {
			if !reflect.DeepEqual(testCase.expectedResult, actualResult) {
				t.Errorf("Test %d: The expected BucketPolicy doesnt match the actual BucketPolicy", i+1)
			}
		}
	}
}
