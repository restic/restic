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
	"crypto/md5"
	"crypto/sha256"
	"errors"
	"hash"
	"io"
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

	// Cleanup any previously left stale files, as the function exits.
	defer cleanupStaleTempfiles("multiparts$-putobject-partial")

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
		partMdata := partMetadata{
			ReadCloser: tmpFile,
			MD5Sum:     hashMD5.Sum(nil),
			Size:       totalReadPartSize,
		}

		// Signature version '4'.
		if c.signature.isV4() {
			partMdata.Sha256Sum = hashSha256.Sum(nil)
		}

		// Current part number to be uploaded.
		partMdata.Number = partNumber

		// execute upload part.
		objPart, err := c.uploadPart(bucketName, objectName, uploadID, partMdata)
		if err != nil {
			// Close the read closer.
			partMdata.ReadCloser.Close()
			return totalUploadedSize, err
		}

		// Save successfully uploaded size.
		totalUploadedSize += partMdata.Size

		// Save successfully uploaded part metadata.
		partsInfo[partMdata.Number] = objPart

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
