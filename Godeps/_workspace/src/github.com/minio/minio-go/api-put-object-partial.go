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
	"errors"
	"fmt"
	"hash"
	"io"
	"io/ioutil"
	"sort"
)

// PutObjectPartial put object partial.
func (c Client) PutObjectPartial(bucketName, objectName string, data ReadAtCloser, size int64, contentType string) (n int64, err error) {
	// Input validation.
	if err := isValidBucketName(bucketName); err != nil {
		return 0, err
	}
	if err := isValidObjectName(objectName); err != nil {
		return 0, err
	}
	// Input size negative should return error.
	if size < 0 {
		return 0, ErrInvalidArgument("Input file size cannot be negative.")
	}
	// Input size bigger than 5TiB should fail.
	if size > int64(maxMultipartPutObjectSize) {
		return 0, ErrInvalidArgument("Input file size is bigger than the supported maximum of 5TiB.")
	}

	// NOTE: Google Cloud Storage does not implement Amazon S3 Compatible multipart PUT.
	// So we fall back to single PUT operation with the maximum limit of 5GiB.
	if isGoogleEndpoint(c.endpointURL) {
		if size > int64(maxSinglePutObjectSize) {
			return 0, ErrorResponse{
				Code:       "NotImplemented",
				Message:    fmt.Sprintf("Invalid Content-Length %d for file uploads to Google Cloud Storage.", size),
				Key:        objectName,
				BucketName: bucketName,
			}
		}
		// Do not compute MD5 for Google Cloud Storage. Uploads upto 5GiB in size.
		n, err := c.putPartialNoChksum(bucketName, objectName, data, size, contentType)
		return n, err
	}

	// NOTE: S3 doesn't allow anonymous multipart requests.
	if isAmazonEndpoint(c.endpointURL) && c.anonymous {
		if size > int64(maxSinglePutObjectSize) {
			return 0, ErrorResponse{
				Code:       "NotImplemented",
				Message:    fmt.Sprintf("For anonymous requests Content-Length cannot be %d.", size),
				Key:        objectName,
				BucketName: bucketName,
			}
		}
		// Do not compute MD5 for anonymous requests to Amazon S3. Uploads upto 5GiB in size.
		n, err := c.putPartialAnonymous(bucketName, objectName, data, size, contentType)
		return n, err
	}

	// Small file upload is initiated for uploads for input data size smaller than 5MiB.
	if size < minimumPartSize {
		n, err = c.putPartialSmallObject(bucketName, objectName, data, size, contentType)
		return n, err
	}
	n, err = c.putPartialLargeObject(bucketName, objectName, data, size, contentType)
	return n, err

}

// putNoChecksumPartial special function used Google Cloud Storage. This special function
// is used for Google Cloud Storage since Google's multipart API is not S3 compatible.
func (c Client) putPartialNoChksum(bucketName, objectName string, data ReadAtCloser, size int64, contentType string) (n int64, err error) {
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

	// Create a new pipe to stage the reads.
	reader, writer := io.Pipe()

	// readAtOffset to carry future offsets.
	var readAtOffset int64

	// readAt defaults to reading at 5MiB buffer.
	readAtBuffer := make([]byte, 1024*1024*5)

	// Initiate a routine to start writing.
	go func() {
		for {
			readAtSize, rerr := data.ReadAt(readAtBuffer, readAtOffset)
			if rerr != nil {
				if rerr != io.EOF {
					writer.CloseWithError(rerr)
					return
				}
			}
			writeSize, werr := writer.Write(readAtBuffer[:readAtSize])
			if werr != nil {
				writer.CloseWithError(werr)
				return
			}
			if readAtSize != writeSize {
				writer.CloseWithError(errors.New("Something really bad happened here. " + reportIssue))
				return
			}
			readAtOffset += int64(writeSize)
			if rerr == io.EOF {
				writer.Close()
				return
			}
		}
	}()
	// For anonymous requests, we will not calculate sha256 and md5sum.
	putObjData := putObjectData{
		MD5Sum:      nil,
		Sha256Sum:   nil,
		ReadCloser:  reader,
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

// putAnonymousPartial is a special function for uploading content as anonymous request.
// This special function is necessary since Amazon S3 doesn't allow anonymous multipart uploads.
func (c Client) putPartialAnonymous(bucketName, objectName string, data ReadAtCloser, size int64, contentType string) (n int64, err error) {
	// Input validation.
	if err := isValidBucketName(bucketName); err != nil {
		return 0, err
	}
	if err := isValidObjectName(objectName); err != nil {
		return 0, err
	}
	return c.putPartialNoChksum(bucketName, objectName, data, size, contentType)
}

// putSmallObjectPartial uploads files smaller than 5MiB.
func (c Client) putPartialSmallObject(bucketName, objectName string, data ReadAtCloser, size int64, contentType string) (n int64, err error) {
	// Input validation.
	if err := isValidBucketName(bucketName); err != nil {
		return 0, err
	}
	if err := isValidObjectName(objectName); err != nil {
		return 0, err
	}

	// readAt defaults to reading at 5MiB buffer.
	readAtBuffer := make([]byte, size)
	readAtSize, err := data.ReadAt(readAtBuffer, 0)
	if err != nil {
		if err != io.EOF {
			return 0, err
		}
	}
	if int64(readAtSize) != size {
		return 0, ErrUnexpectedEOF(int64(readAtSize), size, bucketName, objectName)
	}

	// Construct a new PUT object metadata.
	putObjData := putObjectData{
		MD5Sum:      sumMD5(readAtBuffer),
		Sha256Sum:   sum256(readAtBuffer),
		ReadCloser:  ioutil.NopCloser(bytes.NewReader(readAtBuffer)),
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

// putPartialLargeObject uploads files bigger than 5MiB.
func (c Client) putPartialLargeObject(bucketName, objectName string, data ReadAtCloser, size int64, contentType string) (n int64, err error) {
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
	// previous part number.
	var prevPartNumber int
	// Loop through all parts and calculate totalUploadedSize.
	for _, partInfo := range partsInfo {
		totalUploadedSize += partInfo.Size
		// Choose the maximum part size.
		if partInfo.Size >= prevMaxPartSize {
			prevMaxPartSize = partInfo.Size
		}
		// Save previous part number.
		prevPartNumber = partInfo.PartNumber
	}

	// Calculate the optimal part size for a given file size.
	partSize := optimalPartSize(size)
	// If prevMaxPartSize is set use that.
	if prevMaxPartSize != 0 {
		partSize = prevMaxPartSize
	}

	// MD5 and Sha256 hasher.
	var hashMD5, hashSha256 hash.Hash

	// Part number always starts with prevPartNumber + 1. i.e The next part number.
	partNumber := prevPartNumber + 1

	// Loop through until EOF.
	for totalUploadedSize < size {
		// Initialize a new temporary file.
		tmpFile, err := newTempFile("multiparts$-putobject-partial")
		if err != nil {
			return 0, err
		}

		// Create a hash multiwriter.
		hashMD5 = md5.New()
		hashWriter := io.MultiWriter(hashMD5)
		if c.signature.isV4() {
			hashSha256 = sha256.New()
			hashWriter = io.MultiWriter(hashMD5, hashSha256)
		}
		writer := io.MultiWriter(tmpFile, hashWriter)

		// totalUploadedSize is the current readAtOffset.
		readAtOffset := totalUploadedSize

		// Read until partSize.
		var totalReadPartSize int64

		// readAt defaults to reading at 5MiB buffer.
		readAtBuffer := make([]byte, optimalReadAtBufferSize)

		// Loop through until partSize.
		for totalReadPartSize < partSize {
			readAtSize, rerr := data.ReadAt(readAtBuffer, readAtOffset)
			if rerr != nil {
				if rerr != io.EOF {
					return 0, rerr
				}
			}
			writeSize, werr := writer.Write(readAtBuffer[:readAtSize])
			if werr != nil {
				return 0, werr
			}
			if readAtSize != writeSize {
				return 0, errors.New("Something really bad happened here. " + reportIssue)
			}
			readAtOffset += int64(writeSize)
			totalReadPartSize += int64(writeSize)
			if rerr == io.EOF {
				break
			}
		}

		// Seek back to beginning of the temporary file.
		if _, err := tmpFile.Seek(0, 0); err != nil {
			return 0, err
		}

		// Save all the part metadata.
		prtData := partData{
			ReadCloser: tmpFile,
			MD5Sum:     hashMD5.Sum(nil),
			Size:       totalReadPartSize,
		}

		// Signature version '4'.
		if c.signature.isV4() {
			prtData.Sha256Sum = hashSha256.Sum(nil)
		}

		// Current part number to be uploaded.
		prtData.Number = partNumber

		// execute upload part.
		objPart, err := c.uploadPart(bucketName, objectName, uploadID, prtData)
		if err != nil {
			// Close the read closer.
			prtData.ReadCloser.Close()
			return totalUploadedSize, err
		}

		// Save successfully uploaded size.
		totalUploadedSize += prtData.Size

		// Save successfully uploaded part metadata.
		partsInfo[prtData.Number] = objPart

		// Move to next part.
		partNumber++
	}

	// If size is greater than zero verify totalUploaded.
	// if totalUploaded is different than the input 'size', do not complete the request throw an error.
	if totalUploadedSize != size {
		return totalUploadedSize, ErrUnexpectedEOF(totalUploadedSize, size, bucketName, objectName)
	}

	// Loop over uploaded parts to save them in a Parts array before completing the multipart request.
	for _, part := range partsInfo {
		var complPart completePart
		complPart.ETag = part.ETag
		complPart.PartNumber = part.PartNumber
		completeMultipartUpload.Parts = append(completeMultipartUpload.Parts, complPart)
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
