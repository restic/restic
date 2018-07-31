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

// You know what would be amazing?  If I could autogen this from like a JSON
// file.  Wouldn't that be amazing?  That would be amazing.

const (
	V1api = "/b2api/v1/"
)

type ErrorMessage struct {
	Status int    `json:"status"`
	Code   string `json:"code"`
	Msg    string `json:"message"`
}

type AuthorizeAccountResponse struct {
	AccountID      string    `json:"accountId"`
	AuthToken      string    `json:"authorizationToken"`
	URI            string    `json:"apiUrl"`
	DownloadURI    string    `json:"downloadUrl"`
	MinPartSize    int       `json:"minimumPartSize"`
	PartSize       int       `json:"recommendedPartSize"`
	AbsMinPartSize int       `json:"absoluteMinimumPartSize"`
	Allowed        Allowance `json:"allowed"`
}

type Allowance struct {
	Capabilities []string `json:"capabilities"`
	Bucket       string   `json:"bucketId"`
	Prefix       string   `json:"namePrefix"`
}

type LifecycleRule struct {
	DaysHiddenUntilDeleted int    `json:"daysFromHidingToDeleting,omitempty"`
	DaysNewUntilHidden     int    `json:"daysFromUploadingToHiding,omitempty"`
	Prefix                 string `json:"fileNamePrefix"`
}

type CreateBucketRequest struct {
	AccountID      string            `json:"accountId"`
	Name           string            `json:"bucketName"`
	Type           string            `json:"bucketType"`
	Info           map[string]string `json:"bucketInfo"`
	LifecycleRules []LifecycleRule   `json:"lifecycleRules"`
}

type CreateBucketResponse struct {
	BucketID       string            `json:"bucketId"`
	Name           string            `json:"bucketName"`
	Type           string            `json:"bucketType"`
	Info           map[string]string `json:"bucketInfo"`
	LifecycleRules []LifecycleRule   `json:"lifecycleRules"`
	Revision       int               `json:"revision"`
}

type DeleteBucketRequest struct {
	AccountID string `json:"accountId"`
	BucketID  string `json:"bucketId"`
}

type ListBucketsRequest struct {
	AccountID string `json:"accountId"`
	Bucket    string `json:"bucketId,omitempty"`
}

type ListBucketsResponse struct {
	Buckets []CreateBucketResponse `json:"buckets"`
}

type UpdateBucketRequest struct {
	AccountID      string            `json:"accountId"`
	BucketID       string            `json:"bucketId"`
	Type           string            `json:"bucketType,omitempty"`
	Info           map[string]string `json:"bucketInfo,omitempty"`
	LifecycleRules []LifecycleRule   `json:"lifecycleRules,omitempty"`
	IfRevisionIs   int               `json:"ifRevisionIs,omitempty"`
}

type UpdateBucketResponse CreateBucketResponse

type GetUploadURLRequest struct {
	BucketID string `json:"bucketId"`
}

type GetUploadURLResponse struct {
	URI   string `json:"uploadUrl"`
	Token string `json:"authorizationToken"`
}

type UploadFileResponse GetFileInfoResponse

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
	Prefix       string `json:"prefix,omitempty"`
	Delimiter    string `json:"delimiter,omitempty"`
}

type ListFileNamesResponse struct {
	Continuation string                `json:"nextFileName"`
	Files        []GetFileInfoResponse `json:"files"`
}

type ListFileVersionsRequest struct {
	BucketID  string `json:"bucketId"`
	Count     int    `json:"maxFileCount"`
	StartName string `json:"startFileName,omitempty"`
	StartID   string `json:"startFileId,omitempty"`
	Prefix    string `json:"prefix,omitempty"`
	Delimiter string `json:"delimiter,omitempty"`
}

type ListFileVersionsResponse struct {
	NextName string                `json:"nextFileName"`
	NextID   string                `json:"nextFileId"`
	Files    []GetFileInfoResponse `json:"files"`
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
	FileID      string            `json:"fileId,omitempty"`
	Name        string            `json:"fileName,omitempty"`
	AccountID   string            `json:"accountId,omitempty"`
	BucketID    string            `json:"bucketId,omitempty"`
	Size        int64             `json:"contentLength,omitempty"`
	SHA1        string            `json:"contentSha1,omitempty"`
	ContentType string            `json:"contentType,omitempty"`
	Info        map[string]string `json:"fileInfo,omitempty"`
	Action      string            `json:"action,omitempty"`
	Timestamp   int64             `json:"uploadTimestamp,omitempty"`
}

type GetDownloadAuthorizationRequest struct {
	BucketID           string `json:"bucketId"`
	Prefix             string `json:"fileNamePrefix"`
	Valid              int    `json:"validDurationInSeconds"`
	ContentDisposition string `json:"b2ContentDisposition,omitempty"`
}

type GetDownloadAuthorizationResponse struct {
	BucketID string `json:"bucketId"`
	Prefix   string `json:"fileNamePrefix"`
	Token    string `json:"authorizationToken"`
}

type ListUnfinishedLargeFilesRequest struct {
	BucketID     string `json:"bucketId"`
	Continuation string `json:"startFileId,omitempty"`
	Count        int    `json:"maxFileCount,omitempty"`
}

type ListUnfinishedLargeFilesResponse struct {
	Files        []GetFileInfoResponse `json:"files"`
	Continuation string                `json:"nextFileId"`
}

type CreateKeyRequest struct {
	AccountID    string   `json:"accountId"`
	Capabilities []string `json:"capabilities"`
	Name         string   `json:"keyName"`
	Valid        int      `json:"validDurationInSeconds,omitempty"`
	BucketID     string   `json:"bucketId,omitempty"`
	Prefix       string   `json:"namePrefix,omitempty"`
}

type Key struct {
	ID           string   `json:"applicationKeyId"`
	Secret       string   `json:"applicationKey"`
	AccountID    string   `json:"accountId"`
	Capabilities []string `json:"capabilities"`
	Name         string   `json:"keyName"`
	Expires      int64    `json:"expirationTimestamp"`
	BucketID     string   `json:"bucketId"`
	Prefix       string   `json:"namePrefix"`
}

type CreateKeyResponse Key

type DeleteKeyRequest struct {
	KeyID string `json:"applicationKeyId"`
}

type DeleteKeyResponse Key

type ListKeysRequest struct {
	AccountID string `json:"accountId"`
	Max       int    `json:"maxKeyCount,omitempty"`
	Next      string `json:"startApplicationKeyId,omitempty"`
}

type ListKeysResponse struct {
	Keys []Key  `json:"keys"`
	Next string `json:"nextApplicationKeyId"`
}
