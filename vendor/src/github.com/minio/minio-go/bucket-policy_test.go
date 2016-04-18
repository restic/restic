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
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
)

// Validates bucket policy string.
func TestIsValidBucketPolicy(t *testing.T) {
	testCases := []struct {
		inputPolicy    BucketPolicy
		expectedResult bool
	}{
		// valid inputs.
		{BucketPolicy("none"), true},
		{BucketPolicy("readonly"), true},
		{BucketPolicy("readwrite"), true},
		{BucketPolicy("writeonly"), true},
		// invalid input.
		{BucketPolicy("readwriteonly"), false},
		{BucketPolicy("writeread"), false},
	}

	for i, testCase := range testCases {
		actualResult := testCase.inputPolicy.isValidBucketPolicy()
		if testCase.expectedResult != actualResult {
			t.Errorf("Test %d: Expected IsValidBucket policy to be '%v' for policy \"%s\", but instead found it to be '%v'", i+1, testCase.expectedResult, testCase.inputPolicy, actualResult)
		}
	}
}

// Tests whether first array is completly contained in second array.
func TestSubsetActions(t *testing.T) {
	testCases := []struct {
		firstArray  []string
		secondArray []string

		expectedResult bool
	}{
		{[]string{"aaa", "bbb"}, []string{"ccc", "bbb"}, false},
		{[]string{"aaa", "bbb"}, []string{"aaa", "ccc"}, false},
		{[]string{"aaa", "bbb"}, []string{"aaa", "bbb"}, true},
		{[]string{"aaa", "bbb"}, []string{"aaa", "bbb", "ccc"}, true},
		{[]string{"aaa", "bbb", "aaa"}, []string{"aaa", "bbb", "ccc"}, false},
		{[]string{"aaa", "bbb", "aaa"}, []string{"aaa", "bbb", "bbb", "aaa"}, true},
		{[]string{"aaa", "bbb", "aaa"}, []string{"aaa", "bbb"}, false},
		{[]string{"aaa", "bbb", "aaa"}, []string{"aaa", "bbb", "aaa", "bbb", "ccc"}, true},
	}
	for i, testCase := range testCases {
		actualResult := subsetActions(testCase.firstArray, testCase.secondArray)
		if testCase.expectedResult != actualResult {
			t.Errorf("Test %d: First array '%v' is not contained in second array '%v'", i+1, testCase.firstArray, testCase.secondArray)
		}
	}

}

// Tests validate Bucket Policy type identifier.
func TestIdentifyPolicyType(t *testing.T) {
	testCases := []struct {
		inputPolicy BucketAccessPolicy
		bucketName  string
		objName     string

		expectedPolicy BucketPolicy
	}{
		{BucketAccessPolicy{Version: "2012-10-17"}, "my-bucket", "", BucketPolicyNone},
	}
	for i, testCase := range testCases {
		actualBucketPolicy := identifyPolicyType(testCase.inputPolicy, testCase.bucketName, testCase.objName)
		if testCase.expectedPolicy != actualBucketPolicy {
			t.Errorf("Test %d: Expected bucket policy to be '%v', but instead got '%v'", i+1, testCase.expectedPolicy, actualBucketPolicy)
		}
	}
}

// Test validate Resource Statement Generator.
func TestGeneratePolicyStatement(t *testing.T) {

	testCases := []struct {
		bucketPolicy       BucketPolicy
		bucketName         string
		objectPrefix       string
		expectedStatements []Statement

		shouldPass bool
		err        error
	}{
		{BucketPolicy("my-policy"), "my-bucket", "", []Statement{}, false, ErrInvalidArgument(fmt.Sprintf("Invalid bucket policy provided. %s", BucketPolicy("my-policy")))},
		{BucketPolicyNone, "my-bucket", "", []Statement{}, true, nil},
		{BucketPolicyReadOnly, "read-only-bucket", "", setReadOnlyStatement("read-only-bucket", ""), true, nil},
		{BucketPolicyWriteOnly, "write-only-bucket", "", setWriteOnlyStatement("write-only-bucket", ""), true, nil},
		{BucketPolicyReadWrite, "read-write-bucket", "", setReadWriteStatement("read-write-bucket", ""), true, nil},
	}
	for i, testCase := range testCases {
		actualStatements, err := generatePolicyStatement(testCase.bucketPolicy, testCase.bucketName, testCase.objectPrefix)

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
		// Test passes as expected, but the output values are verified for correctness here.
		if err == nil && testCase.shouldPass {
			if !reflect.DeepEqual(testCase.expectedStatements, actualStatements) {
				t.Errorf("Test %d: The expected statements from resource statement generator doesn't match the actual statements", i+1)
			}
		}
	}
}

// Tests validating read only statement generator.
func TestsetReadOnlyStatement(t *testing.T) {

	expectedReadOnlyStatement := func(bucketName, objectPrefix string) []Statement {
		bucketResourceStatement := &Statement{}
		objectResourceStatement := &Statement{}
		statements := []Statement{}

		bucketResourceStatement.Effect = "Allow"
		bucketResourceStatement.Principal.AWS = []string{"*"}
		bucketResourceStatement.Resources = []string{fmt.Sprintf("%s%s", awsResourcePrefix, bucketName)}
		bucketResourceStatement.Actions = readOnlyBucketActions
		objectResourceStatement.Effect = "Allow"
		objectResourceStatement.Principal.AWS = []string{"*"}
		objectResourceStatement.Resources = []string{fmt.Sprintf("%s%s", awsResourcePrefix, bucketName+"/"+objectPrefix+"*")}
		objectResourceStatement.Actions = readOnlyObjectActions
		// Save the read only policy.
		statements = append(statements, *bucketResourceStatement, *objectResourceStatement)
		return statements
	}

	testCases := []struct {
		// inputs.
		bucketName   string
		objectPrefix string
		// expected result.
		expectedStatements []Statement
	}{
		{"my-bucket", "", expectedReadOnlyStatement("my-bucket", "")},
		{"my-bucket", "Asia/", expectedReadOnlyStatement("my-bucket", "Asia/")},
		{"my-bucket", "Asia/India", expectedReadOnlyStatement("my-bucket", "Asia/India")},
	}

	for i, testCase := range testCases {
		actualStaments := setReadOnlyStatement(testCase.bucketName, testCase.objectPrefix)
		if !reflect.DeepEqual(testCase.expectedStatements, actualStaments) {
			t.Errorf("Test %d: The expected statements from resource statement generator doesn't match the actual statements", i+1)
		}
	}
}

// Tests validating write only statement generator.
func TestsetWriteOnlyStatement(t *testing.T) {

	expectedWriteOnlyStatement := func(bucketName, objectPrefix string) []Statement {
		bucketResourceStatement := &Statement{}
		objectResourceStatement := &Statement{}
		statements := []Statement{}
		// Write only policy.
		bucketResourceStatement.Effect = "Allow"
		bucketResourceStatement.Principal.AWS = []string{"*"}
		bucketResourceStatement.Resources = []string{fmt.Sprintf("%s%s", awsResourcePrefix, bucketName)}
		bucketResourceStatement.Actions = writeOnlyBucketActions
		objectResourceStatement.Effect = "Allow"
		objectResourceStatement.Principal.AWS = []string{"*"}
		objectResourceStatement.Resources = []string{fmt.Sprintf("%s%s", awsResourcePrefix, bucketName+"/"+objectPrefix+"*")}
		objectResourceStatement.Actions = writeOnlyObjectActions
		// Save the write only policy.
		statements = append(statements, *bucketResourceStatement, *objectResourceStatement)
		return statements
	}
	testCases := []struct {
		// inputs.
		bucketName   string
		objectPrefix string
		// expected result.
		expectedStatements []Statement
	}{
		{"my-bucket", "", expectedWriteOnlyStatement("my-bucket", "")},
		{"my-bucket", "Asia/", expectedWriteOnlyStatement("my-bucket", "Asia/")},
		{"my-bucket", "Asia/India", expectedWriteOnlyStatement("my-bucket", "Asia/India")},
	}

	for i, testCase := range testCases {
		actualStaments := setWriteOnlyStatement(testCase.bucketName, testCase.objectPrefix)
		if !reflect.DeepEqual(testCase.expectedStatements, actualStaments) {
			t.Errorf("Test %d: The expected statements from resource statement generator doesn't match the actual statements", i+1)
		}
	}
}

// Tests validating read-write statement generator.
func TestsetReadWriteStatement(t *testing.T) {
	// Obtain statements for read-write BucketPolicy.
	expectedReadWriteStatement := func(bucketName, objectPrefix string) []Statement {
		bucketResourceStatement := &Statement{}
		objectResourceStatement := &Statement{}
		statements := []Statement{}

		bucketResourceStatement.Effect = "Allow"
		bucketResourceStatement.Principal.AWS = []string{"*"}
		bucketResourceStatement.Resources = []string{fmt.Sprintf("%s%s", awsResourcePrefix, bucketName)}
		bucketResourceStatement.Actions = readWriteBucketActions
		objectResourceStatement.Effect = "Allow"
		objectResourceStatement.Principal.AWS = []string{"*"}
		objectResourceStatement.Resources = []string{fmt.Sprintf("%s%s", awsResourcePrefix, bucketName+"/"+objectPrefix+"*")}
		objectResourceStatement.Actions = readWriteObjectActions
		// Save the read write policy.
		statements = append(statements, *bucketResourceStatement, *objectResourceStatement)
		return statements
	}

	testCases := []struct {
		// inputs.
		bucketName   string
		objectPrefix string
		// expected result.
		expectedStatements []Statement
	}{
		{"my-bucket", "", expectedReadWriteStatement("my-bucket", "")},
		{"my-bucket", "Asia/", expectedReadWriteStatement("my-bucket", "Asia/")},
		{"my-bucket", "Asia/India", expectedReadWriteStatement("my-bucket", "Asia/India")},
	}

	for i, testCase := range testCases {
		actualStaments := setReadWriteStatement(testCase.bucketName, testCase.objectPrefix)
		if !reflect.DeepEqual(testCase.expectedStatements, actualStaments) {
			t.Errorf("Test %d: The expected statements from resource statement generator doesn't match the actual statements", i+1)
		}
	}
}

// Tests validate Unmarshalling of BucketAccessPolicy.
func TestUnMarshalBucketPolicy(t *testing.T) {

	bucketAccesPolicies := []BucketAccessPolicy{
		{Version: "1.0"},
		{Version: "1.0", Statements: setReadOnlyStatement("minio-bucket", "")},
		{Version: "1.0", Statements: setReadWriteStatement("minio-bucket", "Asia/")},
		{Version: "1.0", Statements: setWriteOnlyStatement("minio-bucket", "Asia/India/")},
	}

	testCases := []struct {
		inputPolicy BucketAccessPolicy
		// expected results.
		expectedPolicy BucketAccessPolicy
		err            error
		// Flag indicating whether the test should pass.
		shouldPass bool
	}{
		{bucketAccesPolicies[0], bucketAccesPolicies[0], nil, true},
		{bucketAccesPolicies[1], bucketAccesPolicies[1], nil, true},
		{bucketAccesPolicies[2], bucketAccesPolicies[2], nil, true},
		{bucketAccesPolicies[3], bucketAccesPolicies[3], nil, true},
	}
	for i, testCase := range testCases {
		inputPolicyBytes, e := json.Marshal(testCase.inputPolicy)
		if e != nil {
			t.Fatalf("Test %d: Couldn't Marshal bucket policy", i+1)
		}
		actualAccessPolicy, err := unMarshalBucketPolicy(inputPolicyBytes)
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
		// Test passes as expected, but the output values are verified for correctness here.
		if err == nil && testCase.shouldPass {
			if !reflect.DeepEqual(testCase.expectedPolicy, actualAccessPolicy) {
				t.Errorf("Test %d: The expected statements from resource statement generator doesn't match the actual statements", i+1)
			}
		}
	}
}

//  Statement.Action, Statement.Resource, Statement.Principal.AWS fields could be just string also.
// Setting these values to just a string and testing the unMarshalBucketPolicy
func TestUnMarshalBucketPolicyUntyped(t *testing.T) {
	obtainRaw := func(v interface{}, t *testing.T) []byte {
		rawData, e := json.Marshal(v)
		if e != nil {
			t.Fatal(e.Error())
		}
		return rawData
	}

	type untypedStatement struct {
		Sid       string
		Effect    string
		Principal struct {
			AWS json.RawMessage
		}
		Action    json.RawMessage
		Resource  json.RawMessage
		Condition map[string]map[string]string
	}

	type bucketAccessPolicyUntyped struct {
		Version   string
		Statement []untypedStatement
	}

	statements := setReadOnlyStatement("my-bucket", "Asia/")
	expectedBucketPolicy := BucketAccessPolicy{Statements: statements}
	accessPolicyUntyped := bucketAccessPolicyUntyped{}
	accessPolicyUntyped.Statement = make([]untypedStatement, 2)

	accessPolicyUntyped.Statement[0].Effect = statements[0].Effect
	accessPolicyUntyped.Statement[0].Principal.AWS = obtainRaw(statements[0].Principal.AWS, t)
	accessPolicyUntyped.Statement[0].Action = obtainRaw(statements[0].Actions, t)
	accessPolicyUntyped.Statement[0].Resource = obtainRaw(statements[0].Resources, t)

	// Setting the values are strings.
	accessPolicyUntyped.Statement[1].Effect = statements[1].Effect
	accessPolicyUntyped.Statement[1].Principal.AWS = obtainRaw(statements[1].Principal.AWS[0], t)
	accessPolicyUntyped.Statement[1].Action = obtainRaw(statements[1].Actions[0], t)
	accessPolicyUntyped.Statement[1].Resource = obtainRaw(statements[1].Resources[0], t)

	inputPolicyBytes := obtainRaw(accessPolicyUntyped, t)
	actualAccessPolicy, err := unMarshalBucketPolicy(inputPolicyBytes)
	if err != nil {
		t.Fatal("Unmarshalling bucket policy from untyped statements failed")
	}
	if !reflect.DeepEqual(expectedBucketPolicy, actualAccessPolicy) {
		t.Errorf("Expected BucketPolicy after unmarshalling untyped statements doesn't match the actual one")
	}
}

// Tests validate removal of policy statement from the list of statements.
func TestRemoveBucketPolicyStatement(t *testing.T) {
	testCases := []struct {
		bucketName      string
		objectPrefix    string
		inputStatements []Statement
	}{
		{"my-bucket", "", []Statement{}},
		{"read-only-bucket", "", setReadOnlyStatement("read-only-bucket", "")},
		{"write-only-bucket", "", setWriteOnlyStatement("write-only-bucket", "")},
		{"read-write-bucket", "", setReadWriteStatement("read-write-bucket", "")},
	}
	for i, testCase := range testCases {
		actualStatements := removeBucketPolicyStatement(testCase.inputStatements, testCase.bucketName, testCase.objectPrefix)
		// empty statement is expected after the invocation of removeBucketPolicyStatement().
		if len(actualStatements) != 0 {
			t.Errorf("Test %d: The expected statements from resource statement generator doesn't match the actual statements", i+1)
		}
	}
}

// Tests validate removing of read only bucket statement.
func TestRemoveBucketPolicyStatementReadOnly(t *testing.T) {
	var emptyStatement []Statement
	testCases := []struct {
		bucketName         string
		objectPrefix       string
		inputStatements    []Statement
		expectedStatements []Statement
	}{
		{"my-bucket", "", []Statement{}, emptyStatement},
		{"read-only-bucket", "", setReadOnlyStatement("read-only-bucket", ""), emptyStatement},
	}
	for i, testCase := range testCases {
		actualStatements := removeBucketPolicyStatementReadOnly(testCase.inputStatements, testCase.bucketName, testCase.objectPrefix)
		// empty statement is expected after the invocation of removeBucketPolicyStatement().
		if !reflect.DeepEqual(testCase.expectedStatements, actualStatements) {
			t.Errorf("Test %d: Expected policy statements doesn't match the actual one", i+1)
		}
	}
}

// Tests validate removing of write only bucket statement.
func TestRemoveBucketPolicyStatementWriteOnly(t *testing.T) {
	var emptyStatement []Statement
	testCases := []struct {
		bucketName         string
		objectPrefix       string
		inputStatements    []Statement
		expectedStatements []Statement
	}{
		{"my-bucket", "", []Statement{}, emptyStatement},
		{"write-only-bucket", "", setWriteOnlyStatement("write-only-bucket", ""), emptyStatement},
	}
	for i, testCase := range testCases {
		actualStatements := removeBucketPolicyStatementWriteOnly(testCase.inputStatements, testCase.bucketName, testCase.objectPrefix)
		// empty statement is expected after the invocation of removeBucketPolicyStatement().
		if !reflect.DeepEqual(testCase.expectedStatements, actualStatements) {
			t.Errorf("Test %d: Expected policy statements doesn't match the actual one", i+1)
		}
	}
}

// Tests validate removing of read-write bucket statement.
func TestRemoveBucketPolicyStatementReadWrite(t *testing.T) {
	var emptyStatement []Statement
	testCases := []struct {
		bucketName         string
		objectPrefix       string
		inputStatements    []Statement
		expectedStatements []Statement
	}{
		{"my-bucket", "", []Statement{}, emptyStatement},
		{"read-write-bucket", "", setReadWriteStatement("read-write-bucket", ""), emptyStatement},
	}
	for i, testCase := range testCases {
		actualStatements := removeBucketPolicyStatementReadWrite(testCase.inputStatements, testCase.bucketName, testCase.objectPrefix)
		// empty statement is expected after the invocation of removeBucketPolicyStatement().
		if !reflect.DeepEqual(testCase.expectedStatements, actualStatements) {
			t.Errorf("Test %d: Expected policy statements doesn't match the actual one", i+1)
		}
	}
}

// Tests validate whether the bucket policy is read only.
func TestIsBucketPolicyReadOnly(t *testing.T) {
	testCases := []struct {
		bucketName      string
		objectPrefix    string
		inputStatements []Statement
		// expected result.
		expectedResult bool
	}{
		{"my-bucket", "", []Statement{}, false},
		{"read-only-bucket", "", setReadOnlyStatement("read-only-bucket", ""), true},
		{"write-only-bucket", "", setWriteOnlyStatement("write-only-bucket", ""), false},
		{"read-write-bucket", "", setReadWriteStatement("read-write-bucket", ""), true},
	}
	for i, testCase := range testCases {
		actualResult := isBucketPolicyReadOnly(testCase.inputStatements, testCase.bucketName, testCase.objectPrefix)
		// empty statement is expected after the invocation of removeBucketPolicyStatement().
		if testCase.expectedResult != actualResult {
			t.Errorf("Test %d: Expected isBucketPolicyReadonly to '%v', but instead found '%v'", i+1, testCase.expectedResult, actualResult)
		}
	}
}

// Tests validate whether the bucket policy is read-write.
func TestIsBucketPolicyReadWrite(t *testing.T) {
	testCases := []struct {
		bucketName      string
		objectPrefix    string
		inputStatements []Statement
		// expected result.
		expectedResult bool
	}{
		{"my-bucket", "", []Statement{}, false},
		{"read-only-bucket", "", setReadOnlyStatement("read-only-bucket", ""), false},
		{"write-only-bucket", "", setWriteOnlyStatement("write-only-bucket", ""), false},
		{"read-write-bucket", "", setReadWriteStatement("read-write-bucket", ""), true},
	}
	for i, testCase := range testCases {
		actualResult := isBucketPolicyReadWrite(testCase.inputStatements, testCase.bucketName, testCase.objectPrefix)
		// empty statement is expected after the invocation of removeBucketPolicyStatement().
		if testCase.expectedResult != actualResult {
			t.Errorf("Test %d: Expected isBucketPolicyReadonly to '%v', but instead found '%v'", i+1, testCase.expectedResult, actualResult)
		}
	}
}

// Tests validate whether the bucket policy is read only.
func TestIsBucketPolicyWriteOnly(t *testing.T) {
	testCases := []struct {
		bucketName      string
		objectPrefix    string
		inputStatements []Statement
		// expected result.
		expectedResult bool
	}{
		{"my-bucket", "", []Statement{}, false},
		{"read-only-bucket", "", setReadOnlyStatement("read-only-bucket", ""), false},
		{"write-only-bucket", "", setWriteOnlyStatement("write-only-bucket", ""), true},
		{"read-write-bucket", "", setReadWriteStatement("read-write-bucket", ""), true},
	}
	for i, testCase := range testCases {
		actualResult := isBucketPolicyWriteOnly(testCase.inputStatements, testCase.bucketName, testCase.objectPrefix)
		// empty statement is expected after the invocation of removeBucketPolicyStatement().
		if testCase.expectedResult != actualResult {
			t.Errorf("Test %d: Expected isBucketPolicyReadonly to '%v', but instead found '%v'", i+1, testCase.expectedResult, actualResult)
		}
	}
}
