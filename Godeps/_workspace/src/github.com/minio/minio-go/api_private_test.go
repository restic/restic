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
	"strings"
	"testing"
)

func TestACLTypes(t *testing.T) {
	want := map[string]bool{
		"private":            true,
		"public-read":        true,
		"public-read-write":  true,
		"authenticated-read": true,
		"invalid":            false,
	}
	for acl, ok := range want {
		if BucketACL(acl).isValidBucketACL() != ok {
			t.Fatal("Error")
		}
	}
}

func TestUserAgent(t *testing.T) {
	conf := new(Config)
	conf.SetUserAgent("minio", "1.0", "amd64")
	if !strings.Contains(conf.userAgent, "minio") {
		t.Fatalf("Error")
	}
}

func TestGetRegion(t *testing.T) {
	region := getRegion("s3.amazonaws.com")
	if region != "us-east-1" {
		t.Fatalf("Error")
	}
	region = getRegion("localhost:9000")
	if region != "milkyway" {
		t.Fatalf("Error")
	}
}

func TestPartSize(t *testing.T) {
	var maxPartSize int64 = 1024 * 1024 * 1024 * 5
	partSize := calculatePartSize(5000000000000000000)
	if partSize > minimumPartSize {
		if partSize > maxPartSize {
			t.Fatal("invalid result, cannot be bigger than maxPartSize 5GB")
		}
	}
	partSize = calculatePartSize(50000000000)
	if partSize > minimumPartSize {
		t.Fatal("invalid result, cannot be bigger than minimumPartSize 5MB")
	}
}

func TestURLEncoding(t *testing.T) {
	type urlStrings struct {
		name        string
		encodedName string
	}

	want := []urlStrings{
		{
			name:        "bigfile-1._%",
			encodedName: "bigfile-1._%25",
		},
		{
			name:        "本語",
			encodedName: "%E6%9C%AC%E8%AA%9E",
		},
		{
			name:        "本語.1",
			encodedName: "%E6%9C%AC%E8%AA%9E.1",
		},
		{
			name:        ">123>3123123",
			encodedName: "%3E123%3E3123123",
		},
		{
			name:        "test 1 2.txt",
			encodedName: "test%201%202.txt",
		},
		{
			name:        "test++ 1.txt",
			encodedName: "test%2B%2B%201.txt",
		},
	}

	for _, u := range want {
		if u.encodedName != getURLEncodedPath(u.name) {
			t.Errorf("Error")
		}
	}
}
