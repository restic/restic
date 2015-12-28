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
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// GetBucketACL get the permissions on an existing bucket.
//
// Returned values are:
//
//  private - owner gets full access.
//  public-read - owner gets full access, others get read access.
//  public-read-write - owner gets full access, others get full access too.
//  authenticated-read - owner gets full access, authenticated users get read access.
func (c Client) GetBucketACL(bucketName string) (BucketACL, error) {
	if err := isValidBucketName(bucketName); err != nil {
		return "", err
	}

	// Set acl query.
	urlValues := make(url.Values)
	urlValues.Set("acl", "")

	// Instantiate a new request.
	req, err := c.newRequest("GET", requestMetadata{
		bucketName:  bucketName,
		queryValues: urlValues,
	})
	if err != nil {
		return "", err
	}

	// Initiate the request.
	resp, err := c.httpClient.Do(req)
	defer closeResponse(resp)
	if err != nil {
		return "", err
	}
	if resp != nil {
		if resp.StatusCode != http.StatusOK {
			return "", HTTPRespToErrorResponse(resp, bucketName, "")
		}
	}

	// Decode access control policy.
	policy := accessControlPolicy{}
	err = xmlDecoder(resp.Body, &policy)
	if err != nil {
		return "", err
	}

	// We need to avoid following de-serialization check for Google Cloud Storage.
	// On Google Cloud Storage "private" canned ACL's policy do not have grant list.
	// Treat it as a valid case, check for all other vendors.
	if !isGoogleEndpoint(c.endpointURL) {
		if policy.AccessControlList.Grant == nil {
			errorResponse := ErrorResponse{
				Code:            "InternalError",
				Message:         "Access control Grant list is empty. " + reportIssue,
				BucketName:      bucketName,
				RequestID:       resp.Header.Get("x-amz-request-id"),
				HostID:          resp.Header.Get("x-amz-id-2"),
				AmzBucketRegion: resp.Header.Get("x-amz-bucket-region"),
			}
			return "", errorResponse
		}
	}

	// boolean cues to indentify right canned acls.
	var publicRead, publicWrite bool

	// Handle grants.
	grants := policy.AccessControlList.Grant
	for _, g := range grants {
		if g.Grantee.URI == "" && g.Permission == "FULL_CONTROL" {
			continue
		}
		if g.Grantee.URI == "http://acs.amazonaws.com/groups/global/AuthenticatedUsers" && g.Permission == "READ" {
			return BucketACL("authenticated-read"), nil
		} else if g.Grantee.URI == "http://acs.amazonaws.com/groups/global/AllUsers" && g.Permission == "WRITE" {
			publicWrite = true
		} else if g.Grantee.URI == "http://acs.amazonaws.com/groups/global/AllUsers" && g.Permission == "READ" {
			publicRead = true
		}
	}

	// public write and not enabled. return.
	if !publicWrite && !publicRead {
		return BucketACL("private"), nil
	}
	// public write not enabled but public read is. return.
	if !publicWrite && publicRead {
		return BucketACL("public-read"), nil
	}
	// public read and public write are enabled return.
	if publicRead && publicWrite {
		return BucketACL("public-read-write"), nil
	}

	return "", ErrorResponse{
		Code:       "NoSuchBucketPolicy",
		Message:    "The specified bucket does not have a bucket policy.",
		BucketName: bucketName,
		RequestID:  "minio",
	}
}

// GetObject gets object content from specified bucket.
// You may also look at GetPartialObject.
func (c Client) GetObject(bucketName, objectName string) (io.ReadCloser, ObjectStat, error) {
	if err := isValidBucketName(bucketName); err != nil {
		return nil, ObjectStat{}, err
	}
	if err := isValidObjectName(objectName); err != nil {
		return nil, ObjectStat{}, err
	}
	// get the whole object as a stream, no seek or resume supported for this.
	return c.getObject(bucketName, objectName, 0, 0)
}

// ReadAtCloser readat closer interface.
type ReadAtCloser interface {
	io.ReaderAt
	io.Closer
}

// GetObjectPartial returns a io.ReadAt for reading sparse entries.
func (c Client) GetObjectPartial(bucketName, objectName string) (ReadAtCloser, ObjectStat, error) {
	if err := isValidBucketName(bucketName); err != nil {
		return nil, ObjectStat{}, err
	}
	if err := isValidObjectName(objectName); err != nil {
		return nil, ObjectStat{}, err
	}
	// Send an explicit stat to get the actual object size.
	objectStat, err := c.StatObject(bucketName, objectName)
	if err != nil {
		return nil, ObjectStat{}, err
	}

	// Create request channel.
	reqCh := make(chan readAtRequest)
	// Create response channel.
	resCh := make(chan readAtResponse)
	// Create done channel.
	doneCh := make(chan struct{})

	// This routine feeds partial object data as and when the caller reads.
	go func() {
		defer close(reqCh)
		defer close(resCh)

		// Loop through the incoming control messages and read data.
		for {
			select {
			// When the done channel is closed exit our routine.
			case <-doneCh:
				return
			// Request message.
			case req := <-reqCh:
				// Get shortest length.
				// NOTE: Last remaining bytes are usually smaller than
				// req.Buffer size. Use that as the final length.
				length := math.Min(float64(len(req.Buffer)), float64(objectStat.Size-req.Offset))
				httpReader, _, err := c.getObject(bucketName, objectName, req.Offset, int64(length))
				if err != nil {
					resCh <- readAtResponse{
						Error: err,
					}
					return
				}
				size, err := httpReader.Read(req.Buffer)
				resCh <- readAtResponse{
					Size:  size,
					Error: err,
				}
			}
		}
	}()
	// Return the readerAt backed by routine.
	return newObjectReadAtCloser(reqCh, resCh, doneCh, objectStat.Size), objectStat, nil
}

// response message container to reply back for the request.
type readAtResponse struct {
	Size  int
	Error error
}

// request message container to communicate with internal go-routine.
type readAtRequest struct {
	Buffer []byte // requested bytes.
	Offset int64  // readAt offset.
}

// objectReadAtCloser container for io.ReadAtCloser.
type objectReadAtCloser struct {
	// mutex.
	mutex *sync.Mutex

	// User allocated and defined.
	reqCh      chan<- readAtRequest
	resCh      <-chan readAtResponse
	doneCh     chan<- struct{}
	objectSize int64

	// Previous error saved for future calls.
	prevErr error
}

// newObjectReadAtCloser implements a io.ReadSeeker for a HTTP stream.
func newObjectReadAtCloser(reqCh chan<- readAtRequest, resCh <-chan readAtResponse, doneCh chan<- struct{}, objectSize int64) *objectReadAtCloser {
	return &objectReadAtCloser{
		mutex:      new(sync.Mutex),
		reqCh:      reqCh,
		resCh:      resCh,
		doneCh:     doneCh,
		objectSize: objectSize,
	}
}

// ReadAt reads len(b) bytes from the File starting at byte offset off.
// It returns the number of bytes read and the error, if any.
// ReadAt always returns a non-nil error when n < len(b).
// At end of file, that error is io.EOF.
func (r *objectReadAtCloser) ReadAt(p []byte, offset int64) (int, error) {
	// Locking.
	r.mutex.Lock()
	defer r.mutex.Unlock()

	// prevErr is which was saved in previous operation.
	if r.prevErr != nil {
		return 0, r.prevErr
	}

	// Send current information over control channel to indicate we are ready.
	reqMsg := readAtRequest{}

	// Send the current offset and bytes requested.
	reqMsg.Buffer = p
	reqMsg.Offset = offset

	// Send read request over the control channel.
	r.reqCh <- reqMsg

	// Get data over the response channel.
	dataMsg := <-r.resCh

	// Save any error.
	r.prevErr = dataMsg.Error
	if dataMsg.Error != nil {
		if dataMsg.Error == io.EOF {
			return dataMsg.Size, dataMsg.Error
		}
		return 0, dataMsg.Error
	}
	return dataMsg.Size, nil
}

// Closer is the interface that wraps the basic Close method.
//
// The behavior of Close after the first call returns error for
// subsequent Close() calls.
func (r *objectReadAtCloser) Close() (err error) {
	// Locking.
	r.mutex.Lock()
	defer r.mutex.Unlock()

	// prevErr is which was saved in previous operation.
	if r.prevErr != nil {
		return r.prevErr
	}

	// Close successfully.
	close(r.doneCh)

	// Save this for any subsequent frivolous reads.
	errMsg := "objectReadAtCloser: is already closed. Bad file descriptor."
	r.prevErr = errors.New(errMsg)
	return
}

// getObject - retrieve object from Object Storage.
//
// Additionally this function also takes range arguments to download the specified
// range bytes of an object. Setting offset and length = 0 will download the full object.
//
// For more information about the HTTP Range header.
// go to http://www.w3.org/Protocols/rfc2616/rfc2616-sec14.html#sec14.35.
func (c Client) getObject(bucketName, objectName string, offset, length int64) (io.ReadCloser, ObjectStat, error) {
	// Validate input arguments.
	if err := isValidBucketName(bucketName); err != nil {
		return nil, ObjectStat{}, err
	}
	if err := isValidObjectName(objectName); err != nil {
		return nil, ObjectStat{}, err
	}

	customHeader := make(http.Header)
	// Set ranges if length and offset are valid.
	if length > 0 && offset >= 0 {
		customHeader.Set("Range", fmt.Sprintf("bytes=%d-%d", offset, offset+length-1))
	} else if offset > 0 && length == 0 {
		customHeader.Set("Range", fmt.Sprintf("bytes=%d-", offset))
	} else if length < 0 && offset == 0 {
		customHeader.Set("Range", fmt.Sprintf("bytes=%d", length))
	}

	// Instantiate a new request.
	req, err := c.newRequest("GET", requestMetadata{
		bucketName:   bucketName,
		objectName:   objectName,
		customHeader: customHeader,
	})
	if err != nil {
		return nil, ObjectStat{}, err
	}
	// Execute the request.
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, ObjectStat{}, err
	}
	if resp != nil {
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
			return nil, ObjectStat{}, HTTPRespToErrorResponse(resp, bucketName, objectName)
		}
	}
	// trim off the odd double quotes.
	md5sum := strings.Trim(resp.Header.Get("ETag"), "\"")
	// parse the date.
	date, err := time.Parse(http.TimeFormat, resp.Header.Get("Last-Modified"))
	if err != nil {
		msg := "Last-Modified time format not recognized. " + reportIssue
		return nil, ObjectStat{}, ErrorResponse{
			Code:            "InternalError",
			Message:         msg,
			RequestID:       resp.Header.Get("x-amz-request-id"),
			HostID:          resp.Header.Get("x-amz-id-2"),
			AmzBucketRegion: resp.Header.Get("x-amz-bucket-region"),
		}
	}
	contentType := strings.TrimSpace(resp.Header.Get("Content-Type"))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	var objectStat ObjectStat
	objectStat.ETag = md5sum
	objectStat.Key = objectName
	objectStat.Size = resp.ContentLength
	objectStat.LastModified = date
	objectStat.ContentType = contentType

	// do not close body here, caller will close
	return resp.Body, objectStat, nil
}
