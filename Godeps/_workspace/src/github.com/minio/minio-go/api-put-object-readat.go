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
	"hash"
	"io"
	"io/ioutil"
	"sort"
)

// shouldUploadPartReadAt - verify if part should be uploaded.
func shouldUploadPartReadAt(objPart objectPart, objectParts map[int]objectPart) bool {
	// If part not found part should be uploaded.
	uploadedPart, found := objectParts[objPart.PartNumber]
	if !found {
		return true
	}
	// if size mismatches part should be uploaded.
	if uploadedPart.Size != objPart.Size {
		return true
	}
	return false
}

// putObjectMultipartFromReadAt - Uploads files bigger than 5MiB. Supports reader
// of type which implements io.ReaderAt interface (ReadAt method).
//
// NOTE: This function is meant to be used for all readers which
// implement io.ReaderAt which allows us for resuming multipart
// uploads but reading at an offset, which would avoid re-read the
// data which was already uploaded. Internally this function uses
// temporary files for staging all the data, these temporary files are
// cleaned automatically when the caller i.e http client closes the
// stream after uploading all the contents successfully.
func (c Client) putObjectMultipartFromReadAt(bucketName, objectName string, reader io.ReaderAt, size int64, contentType string) (n int64, err error) {
	// Input validation.
	if err := isValidBucketName(bucketName); err != nil {
		return 0, err
	}
	if err := isValidObjectName(objectName); err != nil {
		return 0, err
	}

	// Get upload id for an object, initiates a new multipart request
	// if it cannot find any previously partially uploaded object.
	uploadID, isNew, err := c.getUploadID(bucketName, objectName, contentType)
	if err != nil {
		return 0, err
	}

	// Total data read and written to server. should be equal to 'size' at the end of the call.
	var totalUploadedSize int64

	// Complete multipart upload.
	var completeMultipartUpload completeMultipartUpload

	// A map of all uploaded parts.
	var partsInfo = make(map[int]objectPart)

	// Fetch all parts info previously uploaded.
	if !isNew {
		partsInfo, err = c.listObjectParts(bucketName, objectName, uploadID)
		if err != nil {
			return 0, err
		}
	}

	// Calculate the optimal parts info for a given size.
	totalPartsCount, partSize, lastPartSize, err := optimalPartInfo(size)
	if err != nil {
		return 0, err
	}

	// MD5 and SHA256 hasher.
	var hashMD5, hashSHA256 hash.Hash

	// Used for readability, lastPartNumber is always
	// totalPartsCount.
	lastPartNumber := totalPartsCount

	// partNumber always starts with '1'.
	partNumber := 1

	// Initialize a temporary buffer.
	tmpBuffer := new(bytes.Buffer)

	// Upload all the missing parts.
	for partNumber <= lastPartNumber {
		// Verify object if its uploaded.
		verifyObjPart := objectPart{
			PartNumber: partNumber,
			Size:       partSize,
		}
		// Special case if we see a last part number, save last part
		// size as the proper part size.
		if partNumber == lastPartNumber {
			verifyObjPart = objectPart{
				PartNumber: lastPartNumber,
				Size:       lastPartSize,
			}
		}

		// Verify if part should be uploaded.
		if !shouldUploadPartReadAt(verifyObjPart, partsInfo) {
			// Increment part number when not uploaded.
			partNumber++
			continue
		}

		// If partNumber was not uploaded we calculate the missing
		// part offset and size. For all other part numbers we
		// calculate offset based on multiples of partSize.
		readAtOffset := int64(partNumber-1) * partSize
		missingPartSize := partSize

		// As a special case if partNumber is lastPartNumber, we
		// calculate the offset based on the last part size.
		if partNumber == lastPartNumber {
			readAtOffset = (size - lastPartSize)
			missingPartSize = lastPartSize
		}

		// Create a hash multiwriter.
		hashMD5 = md5.New()
		hashWriter := io.MultiWriter(hashMD5)
		if c.signature.isV4() {
			hashSHA256 = sha256.New()
			hashWriter = io.MultiWriter(hashMD5, hashSHA256)
		}
		writer := io.MultiWriter(tmpBuffer, hashWriter)

		// Read until partSize.
		var totalReadPartSize int64

		// ReadAt defaults to reading at 5MiB buffer.
		readAtBuffer := make([]byte, optimalReadAtBufferSize)

		// Following block reads data at an offset from the input
		// reader and copies data to into local temporary file.
		// Temporary file data is limited to the partSize.
		for totalReadPartSize < missingPartSize {
			readAtSize, rerr := reader.ReadAt(readAtBuffer, readAtOffset)
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

		var md5Sum, sha256Sum []byte
		md5Sum = hashMD5.Sum(nil)
		// Signature version '4'.
		if c.signature.isV4() {
			sha256Sum = hashSHA256.Sum(nil)
		}

		// Proceed to upload the part.
		objPart, err := c.uploadPart(bucketName, objectName, uploadID, ioutil.NopCloser(tmpBuffer),
			partNumber, md5Sum, sha256Sum, totalReadPartSize)
		if err != nil {
			// Reset the buffer upon any error.
			tmpBuffer.Reset()
			return totalUploadedSize, err
		}

		// Save successfully uploaded part metadata.
		partsInfo[partNumber] = objPart

		// Increment part number here after successful part upload.
		partNumber++

		// Reset the buffer.
		tmpBuffer.Reset()
	}

	// Loop over uploaded parts to save them in a Parts array before completing the multipart request.
	for _, part := range partsInfo {
		var complPart completePart
		complPart.ETag = part.ETag
		complPart.PartNumber = part.PartNumber
		totalUploadedSize += part.Size
		completeMultipartUpload.Parts = append(completeMultipartUpload.Parts, complPart)
	}

	// Verify if we uploaded all the data.
	if totalUploadedSize != size {
		return totalUploadedSize, ErrUnexpectedEOF(totalUploadedSize, size, bucketName, objectName)
	}

	// Verify if totalPartsCount is not equal to total list of parts.
	if totalPartsCount != len(completeMultipartUpload.Parts) {
		return totalUploadedSize, ErrInvalidParts(totalPartsCount, len(completeMultipartUpload.Parts))
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
