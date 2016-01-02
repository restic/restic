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
	"bytes"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"hash"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

// completedParts is a collection of parts sortable by their part numbers.
// used for sorting the uploaded parts before completing the multipart request.
type completedParts []completePart

func (a completedParts) Len() int           { return len(a) }
func (a completedParts) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a completedParts) Less(i, j int) bool { return a[i].PartNumber < a[j].PartNumber }

// PutObject creates an object in a bucket.
//
// You must have WRITE permissions on a bucket to create an object.
//
//  - For size smaller than 5MiB PutObject automatically does a single atomic Put operation.
//  - For size larger than 5MiB PutObject automatically does a resumable multipart Put operation.
//  - For size input as -1 PutObject does a multipart Put operation until input stream reaches EOF.
//    Maximum object size that can be uploaded through this operation will be 5TiB.
//
// NOTE: Google Cloud Storage does not implement Amazon S3 Compatible multipart PUT.
// So we fall back to single PUT operation with the maximum limit of 5GiB.
//
// NOTE: For anonymous requests Amazon S3 doesn't allow multipart upload. So we fall back to single PUT operation.
func (c Client) PutObject(bucketName, objectName string, data io.Reader, size int64, contentType string) (n int64, err error) {
	// Input validation.
	if err := isValidBucketName(bucketName); err != nil {
		return 0, err
	}
	if err := isValidObjectName(objectName); err != nil {
		return 0, err
	}

	// NOTE: Google Cloud Storage does not implement Amazon S3 Compatible multipart PUT.
	// So we fall back to single PUT operation with the maximum limit of 5GiB.
	if isGoogleEndpoint(c.endpointURL) {
		if size <= -1 {
			return 0, ErrorResponse{
				Code:       "NotImplemented",
				Message:    "Content-Length cannot be negative for file uploads to Google Cloud Storage.",
				Key:        objectName,
				BucketName: bucketName,
			}
		}
		// Do not compute MD5 for Google Cloud Storage. Uploads upto 5GiB in size.
		return c.putNoChecksum(bucketName, objectName, data, size, contentType)
	}

	// NOTE: S3 doesn't allow anonymous multipart requests.
	if isAmazonEndpoint(c.endpointURL) && c.anonymous {
		if size <= -1 || size > int64(maxSinglePutObjectSize) {
			return 0, ErrorResponse{
				Code:       "NotImplemented",
				Message:    fmt.Sprintf("For anonymous requests Content-Length cannot be %d.", size),
				Key:        objectName,
				BucketName: bucketName,
			}
		}
		// Do not compute MD5 for anonymous requests to Amazon S3. Uploads upto 5GiB in size.
		return c.putAnonymous(bucketName, objectName, data, size, contentType)
	}

	// Large file upload is initiated for uploads for input data size
	// if its greater than 5MiB or data size is negative.
	if size >= minimumPartSize || size < 0 {
		return c.putLargeObject(bucketName, objectName, data, size, contentType)
	}
	return c.putSmallObject(bucketName, objectName, data, size, contentType)
}

// putNoChecksum special function used Google Cloud Storage. This special function
// is used for Google Cloud Storage since Google's multipart API is not S3 compatible.
func (c Client) putNoChecksum(bucketName, objectName string, data io.Reader, size int64, contentType string) (n int64, err error) {
	// Input validation.
	if err := isValidBucketName(bucketName); err != nil {
		return 0, err
	}
	if err := isValidObjectName(objectName); err != nil {
		return 0, err
	}
	if size > maxPartSize {
		return 0, ErrEntityTooLarge(size, bucketName, objectName)
	}
	// For anonymous requests, we will not calculate sha256 and md5sum.
	putObjData := putObjectData{
		MD5Sum:      nil,
		Sha256Sum:   nil,
		ReadCloser:  ioutil.NopCloser(data),
		Size:        size,
		ContentType: contentType,
	}
	// Execute put object.
	st, err := c.putObject(bucketName, objectName, putObjData)
	if err != nil {
		return 0, err
	}
	if st.Size != size {
		return 0, ErrUnexpectedEOF(st.Size, size, bucketName, objectName)
	}
	return size, nil
}

// putAnonymous is a special function for uploading content as anonymous request.
// This special function is necessary since Amazon S3 doesn't allow anonymous
// multipart uploads.
func (c Client) putAnonymous(bucketName, objectName string, data io.Reader, size int64, contentType string) (n int64, err error) {
	// Input validation.
	if err := isValidBucketName(bucketName); err != nil {
		return 0, err
	}
	if err := isValidObjectName(objectName); err != nil {
		return 0, err
	}
	return c.putNoChecksum(bucketName, objectName, data, size, contentType)
}

// putSmallObject uploads files smaller than 5 mega bytes.
func (c Client) putSmallObject(bucketName, objectName string, data io.Reader, size int64, contentType string) (n int64, err error) {
	// Input validation.
	if err := isValidBucketName(bucketName); err != nil {
		return 0, err
	}
	if err := isValidObjectName(objectName); err != nil {
		return 0, err
	}
	// Read input data fully into buffer.
	dataBytes, err := ioutil.ReadAll(data)
	if err != nil {
		return 0, err
	}
	if int64(len(dataBytes)) != size {
		return 0, ErrUnexpectedEOF(int64(len(dataBytes)), size, bucketName, objectName)
	}
	// Construct a new PUT object metadata.
	putObjData := putObjectData{
		MD5Sum:      sumMD5(dataBytes),
		Sha256Sum:   sum256(dataBytes),
		ReadCloser:  ioutil.NopCloser(bytes.NewReader(dataBytes)),
		Size:        size,
		ContentType: contentType,
	}
	// Single part use case, use putObject directly.
	st, err := c.putObject(bucketName, objectName, putObjData)
	if err != nil {
		return 0, err
	}
	if st.Size != size {
		return 0, ErrUnexpectedEOF(st.Size, size, bucketName, objectName)
	}
	return size, nil
}

// hashCopy - calculates Md5sum and Sha256sum for upto partSize amount of bytes.
func (c Client) hashCopy(writer io.ReadWriteSeeker, data io.Reader, partSize int64) (md5Sum, sha256Sum []byte, size int64, err error) {
	// MD5 and Sha256 hasher.
	var hashMD5, hashSha256 hash.Hash
	// MD5 and Sha256 hasher.
	hashMD5 = md5.New()
	hashWriter := io.MultiWriter(writer, hashMD5)
	if c.signature.isV4() {
		hashSha256 = sha256.New()
		hashWriter = io.MultiWriter(writer, hashMD5, hashSha256)
	}

	// Copies to input at writer.
	size, err = io.CopyN(hashWriter, data, partSize)
	if err != nil {
		// If not EOF return error right here.
		if err != io.EOF {
			return nil, nil, 0, err
		}
	}

	// Seek back to beginning of input, any error fail right here.
	if _, err := writer.Seek(0, 0); err != nil {
		return nil, nil, 0, err
	}

	// Finalize md5shum and sha256 sum.
	md5Sum = hashMD5.Sum(nil)
	if c.signature.isV4() {
		sha256Sum = hashSha256.Sum(nil)
	}
	return md5Sum, sha256Sum, size, err
}

// putLargeObject uploads files bigger than 5 mega bytes.
func (c Client) putLargeObject(bucketName, objectName string, data io.Reader, size int64, contentType string) (n int64, err error) {
	// Input validation.
	if err := isValidBucketName(bucketName); err != nil {
		return 0, err
	}
	if err := isValidObjectName(objectName); err != nil {
		return 0, err
	}

	// getUploadID for an object, initiates a new multipart request
	// if it cannot find any previously partially uploaded object.
	uploadID, err := c.getUploadID(bucketName, objectName, contentType)
	if err != nil {
		return 0, err
	}

	// total data read and written to server. should be equal to 'size' at the end of the call.
	var totalUploadedSize int64

	// Complete multipart upload.
	var completeMultipartUpload completeMultipartUpload

	// Fetch previously upload parts and save the total size.
	partsInfo, err := c.listObjectParts(bucketName, objectName, uploadID)
	if err != nil {
		return 0, err
	}
	// Previous maximum part size
	var prevMaxPartSize int64
	// Loop through all parts and calculate totalUploadedSize.
	for _, partInfo := range partsInfo {
		// Choose the maximum part size.
		if partInfo.Size >= prevMaxPartSize {
			prevMaxPartSize = partInfo.Size
		}
	}

	// Calculate the optimal part size for a given size.
	partSize := optimalPartSize(size)
	// If prevMaxPartSize is set use that.
	if prevMaxPartSize != 0 {
		partSize = prevMaxPartSize
	}

	// Part number always starts with '0'.
	partNumber := 0

	// Loop through until EOF.
	for {
		// Increment part number.
		partNumber++

		// Initialize a new temporary file.
		tmpFile, err := newTempFile("multiparts$-putobject")
		if err != nil {
			return 0, err
		}

		// Calculates MD5 and Sha256 sum while copying partSize bytes into tmpFile.
		md5Sum, sha256Sum, size, rErr := c.hashCopy(tmpFile, data, partSize)
		if rErr != nil {
			if rErr != io.EOF {
				return 0, rErr
			}
		}

		// Save all the part metadata.
		prtData := partData{
			ReadCloser: tmpFile,
			Size:       size,
			MD5Sum:     md5Sum,
			Sha256Sum:  sha256Sum,
			Number:     partNumber, // Current part number to be uploaded.
		}

		// If part not uploaded proceed to upload.
		if !isPartUploaded(objectPart{
			ETag:       hex.EncodeToString(prtData.MD5Sum),
			PartNumber: partNumber,
		}, partsInfo) {
			// execute upload part.
			objPart, err := c.uploadPart(bucketName, objectName, uploadID, prtData)
			if err != nil {
				// Close the read closer.
				prtData.ReadCloser.Close()
				return 0, err
			}
			// Save successfully uploaded part metadata.
			partsInfo[prtData.Number] = objPart
		}

		// Close the read closer.
		prtData.ReadCloser.Close()

		// If read error was an EOF, break out of the loop.
		if rErr == io.EOF {
			break
		}
	}

	// Loop over uploaded parts to save them in a Parts array before completing the multipart request.
	for _, part := range partsInfo {
		var complPart completePart
		complPart.ETag = part.ETag
		complPart.PartNumber = part.PartNumber
		completeMultipartUpload.Parts = append(completeMultipartUpload.Parts, complPart)
		// Save successfully uploaded size.
		totalUploadedSize += part.Size
	}

	// If size is greater than zero verify totalUploadedSize. if totalUploadedSize is
	// different than the input 'size', do not complete the request throw an error.
	if size > 0 {
		if totalUploadedSize != size {
			return totalUploadedSize, ErrUnexpectedEOF(totalUploadedSize, size, bucketName, objectName)
		}
	}

	// If partNumber is different than total list of parts, error out.
	if partNumber != len(completeMultipartUpload.Parts) {
		return totalUploadedSize, ErrInvalidParts(partNumber, len(completeMultipartUpload.Parts))
	}

	// Sort all completed parts.
	sort.Sort(completedParts(completeMultipartUpload.Parts))
	_, err = c.completeMultipartUpload(bucketName, objectName, uploadID, completeMultipartUpload)
	if err != nil {
		return totalUploadedSize, err
	}

	// Return final size.
	return totalUploadedSize, nil
}

// putObject - add an object to a bucket.
// NOTE: You must have WRITE permissions on a bucket to add an object to it.
func (c Client) putObject(bucketName, objectName string, putObjData putObjectData) (ObjectStat, error) {
	// Input validation.
	if err := isValidBucketName(bucketName); err != nil {
		return ObjectStat{}, err
	}
	if err := isValidObjectName(objectName); err != nil {
		return ObjectStat{}, err
	}

	if strings.TrimSpace(putObjData.ContentType) == "" {
		putObjData.ContentType = "application/octet-stream"
	}

	// Set headers.
	customHeader := make(http.Header)
	customHeader.Set("Content-Type", putObjData.ContentType)

	// Populate request metadata.
	reqMetadata := requestMetadata{
		bucketName:         bucketName,
		objectName:         objectName,
		customHeader:       customHeader,
		contentBody:        putObjData.ReadCloser,
		contentLength:      putObjData.Size,
		contentSha256Bytes: putObjData.Sha256Sum,
		contentMD5Bytes:    putObjData.MD5Sum,
	}
	// Initiate new request.
	req, err := c.newRequest("PUT", reqMetadata)
	if err != nil {
		return ObjectStat{}, err
	}
	// Execute the request.
	resp, err := c.do(req)
	defer closeResponse(resp)
	if err != nil {
		return ObjectStat{}, err
	}
	if resp != nil {
		if resp.StatusCode != http.StatusOK {
			return ObjectStat{}, HTTPRespToErrorResponse(resp, bucketName, objectName)
		}
	}

	var metadata ObjectStat
	// Trim off the odd double quotes from ETag in the beginning and end.
	metadata.ETag = strings.TrimPrefix(resp.Header.Get("ETag"), "\"")
	metadata.ETag = strings.TrimSuffix(metadata.ETag, "\"")
	// A success here means data was written to server successfully.
	metadata.Size = putObjData.Size

	// Return here.
	return metadata, nil
}

// initiateMultipartUpload initiates a multipart upload and returns an upload ID.
func (c Client) initiateMultipartUpload(bucketName, objectName, contentType string) (initiateMultipartUploadResult, error) {
	// Input validation.
	if err := isValidBucketName(bucketName); err != nil {
		return initiateMultipartUploadResult{}, err
	}
	if err := isValidObjectName(objectName); err != nil {
		return initiateMultipartUploadResult{}, err
	}

	// Initialize url queries.
	urlValues := make(url.Values)
	urlValues.Set("uploads", "")

	if contentType == "" {
		contentType = "application/octet-stream"
	}

	// set ContentType header.
	customHeader := make(http.Header)
	customHeader.Set("Content-Type", contentType)

	reqMetadata := requestMetadata{
		bucketName:   bucketName,
		objectName:   objectName,
		queryValues:  urlValues,
		customHeader: customHeader,
	}

	// Instantiate the request.
	req, err := c.newRequest("POST", reqMetadata)
	if err != nil {
		return initiateMultipartUploadResult{}, err
	}
	// Execute the request.
	resp, err := c.do(req)
	defer closeResponse(resp)
	if err != nil {
		return initiateMultipartUploadResult{}, err
	}
	if resp != nil {
		if resp.StatusCode != http.StatusOK {
			return initiateMultipartUploadResult{}, HTTPRespToErrorResponse(resp, bucketName, objectName)
		}
	}
	// Decode xml initiate multipart.
	initiateMultipartUploadResult := initiateMultipartUploadResult{}
	err = xmlDecoder(resp.Body, &initiateMultipartUploadResult)
	if err != nil {
		return initiateMultipartUploadResult, err
	}
	return initiateMultipartUploadResult, nil
}

// uploadPart uploads a part in a multipart upload.
func (c Client) uploadPart(bucketName, objectName, uploadID string, uploadingPart partData) (objectPart, error) {
	// Input validation.
	if err := isValidBucketName(bucketName); err != nil {
		return objectPart{}, err
	}
	if err := isValidObjectName(objectName); err != nil {
		return objectPart{}, err
	}

	// Get resources properly escaped and lined up before using them in http request.
	urlValues := make(url.Values)
	// Set part number.
	urlValues.Set("partNumber", strconv.Itoa(uploadingPart.Number))
	// Set upload id.
	urlValues.Set("uploadId", uploadID)

	reqMetadata := requestMetadata{
		bucketName:         bucketName,
		objectName:         objectName,
		queryValues:        urlValues,
		contentBody:        uploadingPart.ReadCloser,
		contentLength:      uploadingPart.Size,
		contentSha256Bytes: uploadingPart.Sha256Sum,
		contentMD5Bytes:    uploadingPart.MD5Sum,
	}

	// Instantiate a request.
	req, err := c.newRequest("PUT", reqMetadata)
	if err != nil {
		return objectPart{}, err
	}
	// Execute the request.
	resp, err := c.do(req)
	defer closeResponse(resp)
	if err != nil {
		return objectPart{}, err
	}
	if resp != nil {
		if resp.StatusCode != http.StatusOK {
			return objectPart{}, HTTPRespToErrorResponse(resp, bucketName, objectName)
		}
	}
	// Once successfully uploaded, return completed part.
	objPart := objectPart{}
	objPart.Size = uploadingPart.Size
	objPart.PartNumber = uploadingPart.Number
	// Trim off the odd double quotes from ETag in the beginning and end.
	objPart.ETag = strings.TrimPrefix(resp.Header.Get("ETag"), "\"")
	objPart.ETag = strings.TrimSuffix(objPart.ETag, "\"")
	return objPart, nil
}

// completeMultipartUpload completes a multipart upload by assembling previously uploaded parts.
func (c Client) completeMultipartUpload(bucketName, objectName, uploadID string, complete completeMultipartUpload) (completeMultipartUploadResult, error) {
	// Input validation.
	if err := isValidBucketName(bucketName); err != nil {
		return completeMultipartUploadResult{}, err
	}
	if err := isValidObjectName(objectName); err != nil {
		return completeMultipartUploadResult{}, err
	}

	// Initialize url queries.
	urlValues := make(url.Values)
	urlValues.Set("uploadId", uploadID)

	// Marshal complete multipart body.
	completeMultipartUploadBytes, err := xml.Marshal(complete)
	if err != nil {
		return completeMultipartUploadResult{}, err
	}

	// Instantiate all the complete multipart buffer.
	completeMultipartUploadBuffer := bytes.NewBuffer(completeMultipartUploadBytes)
	reqMetadata := requestMetadata{
		bucketName:         bucketName,
		objectName:         objectName,
		queryValues:        urlValues,
		contentBody:        ioutil.NopCloser(completeMultipartUploadBuffer),
		contentLength:      int64(completeMultipartUploadBuffer.Len()),
		contentSha256Bytes: sum256(completeMultipartUploadBuffer.Bytes()),
	}

	// Instantiate the request.
	req, err := c.newRequest("POST", reqMetadata)
	if err != nil {
		return completeMultipartUploadResult{}, err
	}

	// Execute the request.
	resp, err := c.do(req)
	defer closeResponse(resp)
	if err != nil {
		return completeMultipartUploadResult{}, err
	}
	if resp != nil {
		if resp.StatusCode != http.StatusOK {
			return completeMultipartUploadResult{}, HTTPRespToErrorResponse(resp, bucketName, objectName)
		}
	}
	// If successful response, decode the body.
	completeMultipartUploadResult := completeMultipartUploadResult{}
	err = xmlDecoder(resp.Body, &completeMultipartUploadResult)
	if err != nil {
		return completeMultipartUploadResult, err
	}
	return completeMultipartUploadResult, nil
}
