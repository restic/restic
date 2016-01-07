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
	"hash"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
)

// Verify if reader is *os.File
func isFile(reader io.Reader) (ok bool) {
	_, ok = reader.(*os.File)
	return
}

// Verify if reader is *minio.Object
func isObject(reader io.Reader) (ok bool) {
	_, ok = reader.(*Object)
	return
}

// Verify if reader is a generic ReaderAt
func isReadAt(reader io.Reader) (ok bool) {
	_, ok = reader.(io.ReaderAt)
	return
}

// hashCopyN - Calculates Md5sum and SHA256sum for upto partSize amount of bytes.
func (c Client) hashCopyN(writer io.ReadWriteSeeker, reader io.Reader, partSize int64) (md5Sum, sha256Sum []byte, size int64, err error) {
	// MD5 and SHA256 hasher.
	var hashMD5, hashSHA256 hash.Hash
	// MD5 and SHA256 hasher.
	hashMD5 = md5.New()
	hashWriter := io.MultiWriter(writer, hashMD5)
	if c.signature.isV4() {
		hashSHA256 = sha256.New()
		hashWriter = io.MultiWriter(writer, hashMD5, hashSHA256)
	}

	// Copies to input at writer.
	size, err = io.CopyN(hashWriter, reader, partSize)
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
		sha256Sum = hashSHA256.Sum(nil)
	}
	return md5Sum, sha256Sum, size, err
}

// Comprehensive put object operation involving multipart resumable uploads.
//
// Following code handles these types of readers.
//
//  - *os.File
//  - *minio.Object
//  - Any reader which has a method 'ReadAt()'
//
// If we exhaust all the known types, code proceeds to use stream as
// is where each part is re-downloaded, checksummed and verified
// before upload.
func (c Client) putObjectMultipart(bucketName, objectName string, reader io.Reader, size int64, contentType string) (n int64, err error) {
	if size > 0 && size >= minimumPartSize {
		// Verify if reader is *os.File, then use file system functionalities.
		if isFile(reader) {
			return c.putObjectMultipartFromFile(bucketName, objectName, reader.(*os.File), size, contentType)
		}
		// Verify if reader is *minio.Object or io.ReaderAt.
		// NOTE: Verification of object is kept for a specific purpose
		// while it is going to be duck typed similar to io.ReaderAt.
		// It is to indicate that *minio.Object implements io.ReaderAt.
		// and such a functionality is used in the subsequent code
		// path.
		if isObject(reader) || isReadAt(reader) {
			return c.putObjectMultipartFromReadAt(bucketName, objectName, reader.(io.ReaderAt), size, contentType)
		}
	}
	// For any other data size and reader type we do generic multipart
	// approach by staging data in temporary files and uploading them.
	return c.putObjectMultipartStream(bucketName, objectName, reader, size, contentType)
}

// putObjectStream uploads files bigger than 5MiB, and also supports
// special case where size is unknown i.e '-1'.
func (c Client) putObjectMultipartStream(bucketName, objectName string, reader io.Reader, size int64, contentType string) (n int64, err error) {
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

	// Total data read and written to server. should be equal to 'size' at the end of the call.
	var totalUploadedSize int64

	// Complete multipart upload.
	var completeMultipartUpload completeMultipartUpload

	// Fetch previously upload parts.
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
	// Use prevMaxPartSize if available.
	if prevMaxPartSize != 0 {
		partSize = prevMaxPartSize
	}

	// Part number always starts with '0'.
	partNumber := 0

	// Upload each part until EOF.
	for {
		// Increment part number.
		partNumber++

		// Initialize a new temporary file.
		tmpFile, err := newTempFile("multiparts$-putobject-stream")
		if err != nil {
			return 0, err
		}

		// Calculates MD5 and SHA256 sum while copying partSize bytes into tmpFile.
		md5Sum, sha256Sum, size, rErr := c.hashCopyN(tmpFile, reader, partSize)
		if rErr != nil {
			if rErr != io.EOF {
				return 0, rErr
			}
		}

		// Verify if part was not uploaded.
		if !isPartUploaded(objectPart{
			ETag:       hex.EncodeToString(md5Sum),
			PartNumber: partNumber,
		}, partsInfo) {
			// Proceed to upload the part.
			objPart, err := c.uploadPart(bucketName, objectName, uploadID, tmpFile, partNumber, md5Sum, sha256Sum, size)
			if err != nil {
				// Close the temporary file upon any error.
				tmpFile.Close()
				return 0, err
			}
			// Save successfully uploaded part metadata.
			partsInfo[partNumber] = objPart
		}

		// Close the temporary file.
		tmpFile.Close()

		// If read error was an EOF, break out of the loop.
		if rErr == io.EOF {
			break
		}
	}

	// Verify if we uploaded all the data.
	if size > 0 {
		if totalUploadedSize != size {
			return totalUploadedSize, ErrUnexpectedEOF(totalUploadedSize, size, bucketName, objectName)
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

	// Verify if partNumber is different than total list of parts.
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

// initiateMultipartUpload - Initiates a multipart upload and returns an upload ID.
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

	// Set ContentType header.
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
	// Decode xml for new multipart upload.
	initiateMultipartUploadResult := initiateMultipartUploadResult{}
	err = xmlDecoder(resp.Body, &initiateMultipartUploadResult)
	if err != nil {
		return initiateMultipartUploadResult, err
	}
	return initiateMultipartUploadResult, nil
}

// uploadPart - Uploads a part in a multipart upload.
func (c Client) uploadPart(bucketName, objectName, uploadID string, reader io.ReadCloser, partNumber int, md5Sum, sha256Sum []byte, size int64) (objectPart, error) {
	// Input validation.
	if err := isValidBucketName(bucketName); err != nil {
		return objectPart{}, err
	}
	if err := isValidObjectName(objectName); err != nil {
		return objectPart{}, err
	}
	if size > maxPartSize {
		return objectPart{}, ErrEntityTooLarge(size, bucketName, objectName)
	}
	if size <= -1 {
		return objectPart{}, ErrEntityTooSmall(size, bucketName, objectName)
	}
	if partNumber <= 0 {
		return objectPart{}, ErrInvalidArgument("Part number cannot be negative or equal to zero.")
	}
	if uploadID == "" {
		return objectPart{}, ErrInvalidArgument("UploadID cannot be empty.")
	}

	// Get resources properly escaped and lined up before using them in http request.
	urlValues := make(url.Values)
	// Set part number.
	urlValues.Set("partNumber", strconv.Itoa(partNumber))
	// Set upload id.
	urlValues.Set("uploadId", uploadID)

	reqMetadata := requestMetadata{
		bucketName:         bucketName,
		objectName:         objectName,
		queryValues:        urlValues,
		contentBody:        reader,
		contentLength:      size,
		contentMD5Bytes:    md5Sum,
		contentSHA256Bytes: sha256Sum,
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
	objPart.Size = size
	objPart.PartNumber = partNumber
	// Trim off the odd double quotes from ETag in the beginning and end.
	objPart.ETag = strings.TrimPrefix(resp.Header.Get("ETag"), "\"")
	objPart.ETag = strings.TrimSuffix(objPart.ETag, "\"")
	return objPart, nil
}

// completeMultipartUpload - Completes a multipart upload by assembling previously uploaded parts.
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
		contentSHA256Bytes: sum256(completeMultipartUploadBuffer.Bytes()),
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
	// Decode completed multipart upload response on success.
	completeMultipartUploadResult := completeMultipartUploadResult{}
	err = xmlDecoder(resp.Body, &completeMultipartUploadResult)
	if err != nil {
		return completeMultipartUploadResult, err
	}
	return completeMultipartUploadResult, nil
}
