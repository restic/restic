// Copyright 2016, Google
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package b2types implements internal types common to the B2 API.
package b2types

const (
	V1api = "/b2api/v1/"
)

type ErrorMessage struct {
	Status int    `json:"status"`
	Code   string `json:"code"`
	Msg    string `json:"message"`
}

type AuthorizeAccountResponse struct {
	AccountID   string `json:"accountId"`
	AuthToken   string `json:"authorizationToken"`
	URI         string `json:"apiUrl"`
	DownloadURI string `json:"downloadUrl"`
	MinPartSize int    `json:"minimumPartSize"`
}

type CreateBucketRequest struct {
	AccountID string `json:"accountId"`
	Name      string `json:"bucketName"`
	Type      string `json:"bucketType"`
}

type CreateBucketResponse struct {
	BucketID string `json:"bucketId"`
}

type DeleteBucketRequest struct {
	AccountID string `json:"accountId"`
	BucketID  string `json:"bucketId"`
}

type ListBucketsRequest struct {
	AccountID string `json:"accountId"`
}

type ListBucketsResponse struct {
	Buckets []struct {
		BucketID   string `json:"bucketId"`
		BucketName string `json:"bucketName"`
		BucketType string `json:"bucketType"`
	} `json:"buckets"`
}

type GetUploadURLRequest struct {
	BucketID string `json:"bucketId"`
}

type GetUploadURLResponse struct {
	URI   string `json:"uploadUrl"`
	Token string `json:"authorizationToken"`
}

type UploadFileResponse struct {
	FileID    string `json:"fileId"`
	Timestamp int64  `json:"uploadTimestamp"`
	Action    string `json:"action"`
}

type DeleteFileVersionRequest struct {
	Name   string `json:"fileName"`
	FileID string `json:"fileId"`
}

type StartLargeFileRequest struct {
	BucketID    string            `json:"bucketId"`
	Name        string            `json:"fileName"`
	ContentType string            `json:"contentType"`
	Info        map[string]string `json:"fileInfo,omitempty"`
}

type StartLargeFileResponse struct {
	ID string `json:"fileId"`
}

type CancelLargeFileRequest struct {
	ID string `json:"fileId"`
}

type ListPartsRequest struct {
	ID    string `json:"fileId"`
	Start int    `json:"startPartNumber"`
	Count int    `json:"maxPartCount"`
}

type ListPartsResponse struct {
	Next  int `json:"nextPartNumber"`
	Parts []struct {
		ID     string `json:"fileId"`
		Number int    `json:"partNumber"`
		SHA1   string `json:"contentSha1"`
		Size   int64  `json:"contentLength"`
	} `json:"parts"`
}

type getUploadPartURLRequest struct {
	ID string `json:"fileId"`
}

type getUploadPartURLResponse struct {
	URL   string `json:"uploadUrl"`
	Token string `json:"authorizationToken"`
}

type FinishLargeFileRequest struct {
	ID     string   `json:"fileId"`
	Hashes []string `json:"partSha1Array"`
}

type FinishLargeFileResponse struct {
	Name      string `json:"fileName"`
	FileID    string `json:"fileId"`
	Timestamp int64  `json:"uploadTimestamp"`
	Action    string `json:"action"`
}

type ListFileNamesRequest struct {
	BucketID     string `json:"bucketId"`
	Count        int    `json:"maxFileCount"`
	Continuation string `json:"startFileName,omitempty"`
}

type ListFileNamesResponse struct {
	Continuation string `json:"nextFileName"`
	Files        []struct {
		FileID    string `json:"fileId"`
		Name      string `json:"fileName"`
		Size      int64  `json:"size"`
		Action    string `json:"action"`
		Timestamp int64  `json:"uploadTimestamp"`
	} `json:"files"`
}

type ListFileVersionsRequest struct {
	BucketID  string `json:"bucketId"`
	Count     int    `json:"maxFileCount"`
	StartName string `json:"startFileName,omitempty"`
	StartID   string `json:"startFileId,omitempty"`
}

type ListFileVersionsResponse struct {
	NextName string `json:"nextFileName"`
	NextID   string `json:"nextFileId"`
	Files    []struct {
		FileID    string `json:"fileId"`
		Name      string `json:"fileName"`
		Size      int64  `json:"size"`
		Action    string `json:"action"`
		Timestamp int64  `json:"uploadTimestamp"`
	} `json:"files"`
}

type HideFileRequest struct {
	BucketID string `json:"bucketId"`
	File     string `json:"fileName"`
}

type HideFileResponse struct {
	ID        string `json:"fileId"`
	Timestamp int64  `json:"uploadTimestamp"`
	Action    string `json:"action"`
}

type GetFileInfoRequest struct {
	ID string `json:"fileId"`
}

type GetFileInfoResponse struct {
	Name        string            `json:"fileName"`
	SHA1        string            `json:"contentSha1"`
	Size        int64             `json:"contentLength"`
	ContentType string            `json:"contentType"`
	Info        map[string]string `json:"fileInfo"`
	Action      string            `json:"action"`
	Timestamp   int64             `json:"uploadTimestamp"`
}

type GetDownloadAuthorizationRequest struct {
	BucketID string `json:"bucketId"`
	Prefix   string `json:"fileNamePrefix"`
	Valid    int    `json:"validDurationInSeconds"`
}

type GetDownloadAuthorizationResponse struct {
	BucketID string `json:"bucketId"`
	Prefix   string `json:"fileNamePrefix"`
	Token    string `json:"authorizationToken"`
}
