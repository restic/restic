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
	// Previous part number.
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

	// MD5 and SHA256 hasher.
	var hashMD5, hashSHA256 hash.Hash

	// Part number always starts with prevPartNumber + 1. i.e The next part number.
	partNumber := prevPartNumber + 1

	// Upload each part until totalUploadedSize reaches input reader size.
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
			hashSHA256 = sha256.New()
			hashWriter = io.MultiWriter(hashMD5, hashSHA256)
		}
		writer := io.MultiWriter(tmpFile, hashWriter)

		// Choose totalUploadedSize as the current readAtOffset.
		readAtOffset := totalUploadedSize

		// Read until partSize.
		var totalReadPartSize int64

		// ReadAt defaults to reading at 5MiB buffer.
		readAtBuffer := make([]byte, optimalReadAtBufferSize)

		// Following block reads data at an offset from the input
		// reader and copies data to into local temporary file.
		// Temporary file data is limited to the partSize.
		for totalReadPartSize < partSize {
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

		// Seek back to beginning of the temporary file.
		if _, err := tmpFile.Seek(0, 0); err != nil {
			return 0, err
		}

		var md5Sum, sha256Sum []byte
		md5Sum = hashMD5.Sum(nil)
		// Signature version '4'.
		if c.signature.isV4() {
			sha256Sum = hashSHA256.Sum(nil)
		}

		// Proceed to upload the part.
		objPart, err := c.uploadPart(bucketName, objectName, uploadID, tmpFile, partNumber, md5Sum, sha256Sum, totalReadPartSize)
		if err != nil {
			// Close the read closer.
			tmpFile.Close()
			return totalUploadedSize, err
		}

		// Save successfully uploaded size.
		totalUploadedSize += totalReadPartSize

		// Save successfully uploaded part metadata.
		partsInfo[partNumber] = objPart

		// Move to next part.
		partNumber++
	}

	// Verify if we uploaded all the data.
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
