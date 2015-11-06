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
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"io"
	"strings"
	"time"
)

// decoder provides a unified decoding method interface
type decoder interface {
	Decode(v interface{}) error
}

// acceptTypeDecoder provide decoded value in given acceptType
func acceptTypeDecoder(body io.Reader, acceptType string, v interface{}) error {
	var d decoder
	switch {
	case acceptType == "application/xml":
		d = xml.NewDecoder(body)
	case acceptType == "application/json":
		d = json.NewDecoder(body)
	default:
		d = xml.NewDecoder(body)
	}
	return d.Decode(v)
}

// sum256Reader calculate sha256 sum for an input read seeker
func sum256Reader(reader io.ReadSeeker) ([]byte, error) {
	h := sha256.New()
	var err error

	start, _ := reader.Seek(0, 1)
	defer reader.Seek(start, 0)

	for err == nil {
		length := 0
		byteBuffer := make([]byte, 1024*1024)
		length, err = reader.Read(byteBuffer)
		byteBuffer = byteBuffer[0:length]
		h.Write(byteBuffer)
	}

	if err != io.EOF {
		return nil, err
	}

	return h.Sum(nil), nil
}

// sum256 calculate sha256 sum for an input byte array
func sum256(data []byte) []byte {
	hash := sha256.New()
	hash.Write(data)
	return hash.Sum(nil)
}

// sumHMAC calculate hmac between two input byte array
func sumHMAC(key []byte, data []byte) []byte {
	hash := hmac.New(sha256.New, key)
	hash.Write(data)
	return hash.Sum(nil)
}

// getSigningKey hmac seed to calculate final signature
func getSigningKey(secret, region string, t time.Time) []byte {
	date := sumHMAC([]byte("AWS4"+secret), []byte(t.Format(yyyymmdd)))
	regionbytes := sumHMAC(date, []byte(region))
	service := sumHMAC(regionbytes, []byte("s3"))
	signingKey := sumHMAC(service, []byte("aws4_request"))
	return signingKey
}

// getSignature final signature in hexadecimal form
func getSignature(signingKey []byte, stringToSign string) string {
	return hex.EncodeToString(sumHMAC(signingKey, []byte(stringToSign)))
}

// getScope generate a string of a specific date, an AWS region, and a service
func getScope(region string, t time.Time) string {
	scope := strings.Join([]string{
		t.Format(yyyymmdd),
		region,
		"s3",
		"aws4_request",
	}, "/")
	return scope
}

// getCredential generate a credential string
func getCredential(accessKeyID, region string, t time.Time) string {
	scope := getScope(region, t)
	return accessKeyID + "/" + scope
}
