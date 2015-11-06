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

package minio_test

// bucketHandler is an http.Handler that verifies bucket responses and validates incoming requests
import (
	"bytes"
	"io"
	"net/http"
	"strconv"
	"time"
)

type bucketHandler struct {
	resource string
}

func (h bucketHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == "GET":
		switch {
		case r.URL.Path == "/":
			response := []byte("<?xml version=\"1.0\" encoding=\"UTF-8\"?><ListAllMyBucketsResult xmlns=\"http://s3.amazonaws.com/doc/2006-03-01/\"><Buckets><Bucket><Name>bucket</Name><CreationDate>2015-05-20T23:05:09.230Z</CreationDate></Bucket></Buckets><Owner><ID>minio</ID><DisplayName>minio</DisplayName></Owner></ListAllMyBucketsResult>")
			w.Header().Set("Content-Length", strconv.Itoa(len(response)))
			w.Write(response)
		case r.URL.Path == "/bucket":
			_, ok := r.URL.Query()["acl"]
			if ok {
				response := []byte("<?xml version=\"1.0\" encoding=\"UTF-8\"?><AccessControlPolicy><Owner><ID>75aa57f09aa0c8caeab4f8c24e99d10f8e7faeebf76c078efc7c6caea54ba06a</ID><DisplayName>CustomersName@amazon.com</DisplayName></Owner><AccessControlList><Grant><Grantee xmlns:xsi=\"http://www.w3.org/2001/XMLSchema-instance\" xsi:type=\"CanonicalUser\"><ID>75aa57f09aa0c8caeab4f8c24e99d10f8e7faeebf76c078efc7c6caea54ba06a</ID><DisplayName>CustomersName@amazon.com</DisplayName></Grantee><Permission>FULL_CONTROL</Permission></Grant></AccessControlList></AccessControlPolicy>")
				w.Header().Set("Content-Length", strconv.Itoa(len(response)))
				w.Write(response)
				return
			}
			fallthrough
		case r.URL.Path == "/bucket":
			response := []byte("<?xml version=\"1.0\" encoding=\"UTF-8\"?><ListBucketResult xmlns=\"http://s3.amazonaws.com/doc/2006-03-01/\"><Contents><ETag>\"259d04a13802ae09c7e41be50ccc6baa\"</ETag><Key>object</Key><LastModified>2015-05-21T18:24:21.097Z</LastModified><Size>22061</Size><Owner><ID>minio</ID><DisplayName>minio</DisplayName></Owner><StorageClass>STANDARD</StorageClass></Contents><Delimiter></Delimiter><EncodingType></EncodingType><IsTruncated>false</IsTruncated><Marker></Marker><MaxKeys>1000</MaxKeys><Name>testbucket</Name><NextMarker></NextMarker><Prefix></Prefix></ListBucketResult>")
			w.Header().Set("Content-Length", strconv.Itoa(len(response)))
			w.Write(response)
		}
	case r.Method == "PUT":
		switch {
		case r.URL.Path == h.resource:
			_, ok := r.URL.Query()["acl"]
			if ok {
				switch r.Header.Get("x-amz-acl") {
				case "public-read-write":
					fallthrough
				case "public-read":
					fallthrough
				case "private":
					fallthrough
				case "authenticated-read":
					w.WriteHeader(http.StatusOK)
					return
				default:
					w.WriteHeader(http.StatusNotImplemented)
					return
				}
			}
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusBadRequest)
		}
	case r.Method == "HEAD":
		switch {
		case r.URL.Path == h.resource:
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusForbidden)
		}
	case r.Method == "DELETE":
		switch {
		case r.URL.Path != h.resource:
			w.WriteHeader(http.StatusNotFound)
		default:
			h.resource = ""
			w.WriteHeader(http.StatusNoContent)
		}
	}
}

// objectHandler is an http.Handler that verifies object responses and validates incoming requests
type objectHandler struct {
	resource string
	data     []byte
}

func (h objectHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == "PUT":
		length, err := strconv.Atoi(r.Header.Get("Content-Length"))
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		var buffer bytes.Buffer
		_, err = io.CopyN(&buffer, r.Body, int64(length))
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if !bytes.Equal(h.data, buffer.Bytes()) {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("ETag", "\"9af2f8218b150c351ad802c6f3d66abe\"")
		w.WriteHeader(http.StatusOK)
	case r.Method == "HEAD":
		if r.URL.Path != h.resource {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Length", strconv.Itoa(len(h.data)))
		w.Header().Set("Last-Modified", time.Now().UTC().Format(http.TimeFormat))
		w.Header().Set("ETag", "\"9af2f8218b150c351ad802c6f3d66abe\"")
		w.WriteHeader(http.StatusOK)
	case r.Method == "POST":
		_, ok := r.URL.Query()["uploads"]
		if ok {
			response := []byte("<?xml version=\"1.0\" encoding=\"UTF-8\"?><InitiateMultipartUploadResult xmlns=\"http://s3.amazonaws.com/doc/2006-03-01/\"><Bucket>example-bucket</Bucket><Key>object</Key><UploadId>XXBsb2FkIElEIGZvciBlbHZpbmcncyVcdS1tb3ZpZS5tMnRzEEEwbG9hZA</UploadId></InitiateMultipartUploadResult>")
			w.Header().Set("Content-Length", strconv.Itoa(len(response)))
			w.Write(response)
			return
		}
	case r.Method == "GET":
		_, ok := r.URL.Query()["uploadId"]
		if ok {
			uploadID := r.URL.Query().Get("uploadId")
			if uploadID != "XXBsb2FkIElEIGZvciBlbHZpbmcncyVcdS1tb3ZpZS5tMnRzEEEwbG9hZA" {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			response := []byte("<?xml version=\"1.0\" encoding=\"UTF-8\"?><ListPartsResult xmlns=\"http://s3.amazonaws.com/doc/2006-03-01/\"><Bucket>example-bucket</Bucket><Key>example-object</Key><UploadId>XXBsb2FkIElEIGZvciBlbHZpbmcncyVcdS1tb3ZpZS5tMnRzEEEwbG9hZA</UploadId><Initiator><ID>arn:aws:iam::111122223333:user/some-user-11116a31-17b5-4fb7-9df5-b288870f11xx</ID><DisplayName>umat-user-11116a31-17b5-4fb7-9df5-b288870f11xx</DisplayName></Initiator><Owner><ID>75aa57f09aa0c8caeab4f8c24e99d10f8e7faeebf76c078efc7c6caea54ba06a</ID><DisplayName>someName</DisplayName></Owner><StorageClass>STANDARD</StorageClass><PartNumberMarker>1</PartNumberMarker><NextPartNumberMarker>3</NextPartNumberMarker><MaxParts>2</MaxParts><IsTruncated>true</IsTruncated><Part><PartNumber>2</PartNumber><LastModified>2010-11-10T20:48:34.000Z</LastModified><ETag>\"7778aef83f66abc1fa1e8477f296d394\"</ETag><Size>10485760</Size></Part><Part><PartNumber>3</PartNumber><LastModified>2010-11-10T20:48:33.000Z</LastModified><ETag>\"aaaa18db4cc2f85cedef654fccc4a4x8\"</ETag><Size>10485760</Size></Part></ListPartsResult>")
			w.Header().Set("Content-Length", strconv.Itoa(len(response)))
			w.Write(response)
			return
		}
		if r.URL.Path != h.resource {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Length", strconv.Itoa(len(h.data)))
		w.Header().Set("Last-Modified", time.Now().UTC().Format(http.TimeFormat))
		w.Header().Set("ETag", "\"9af2f8218b150c351ad802c6f3d66abe\"")
		w.WriteHeader(http.StatusOK)
		io.Copy(w, bytes.NewReader(h.data))
	case r.Method == "DELETE":
		if r.URL.Path != h.resource {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		h.resource = ""
		h.data = nil
		w.WriteHeader(http.StatusNoContent)
	}
}
