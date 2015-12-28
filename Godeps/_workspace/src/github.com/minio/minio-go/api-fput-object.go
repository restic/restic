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
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"io/ioutil"
	"os"
	"sort"
)

// getUploadID if already present for object name or initiate a request to fetch a new upload id.
func (c Client) getUploadID(bucketName, objectName, contentType string) (string, error) {
	// Input validation.
	if err := isValidBucketName(bucketName); err != nil {
		return "", err
	}
	if err := isValidObjectName(objectName); err != nil {
		return "", err
	}

	// Set content Type to default if empty string.
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	// Find upload id for previous upload for an object.
	uploadID, err := c.findUploadID(bucketName, objectName)
	if err != nil {
		return "", err
	}
	if uploadID == "" {
		// Initiate multipart upload for an object.
		initMultipartUploadResult, err := c.initiateMultipartUpload(bucketName, objectName, contentType)
		if err != nil {
			return "", err
		}
		// Save the new upload id.
		uploadID = initMultipartUploadResult.UploadID
	}
	return uploadID, nil
}

// FPutObject - put object a file.
func (c Client) FPutObject(bucketName, objectName, filePath, contentType string) (int64, error) {
	// Input validation.
	if err := isValidBucketName(bucketName); err != nil {
		return 0, err
	}
	if err := isValidObjectName(objectName); err != nil {
		return 0, err
	}

	// Open the referenced file.
	fileData, err := os.Open(filePath)
	// If any error fail quickly here.
	if err != nil {
		return 0, err
	}
	defer fileData.Close()

	// Save the file stat.
	fileStat, err := fileData.Stat()
	if err != nil {
		return 0, err
	}

	// Save the file size.
	fileSize := fileStat.Size()
	if fileSize > int64(maxMultipartPutObjectSize) {
		return 0, ErrInvalidArgument("Input file size is bigger than the supported maximum of 5TiB.")
	}

	// NOTE: Google Cloud Storage multipart Put is not compatible with Amazon S3 APIs.
	// Current implementation will only upload a maximum of 5GiB to Google Cloud Storage servers.
	if isGoogleEndpoint(c.endpointURL) {
		if fileSize <= -1 || fileSize > int64(maxSinglePutObjectSize) {
			return 0, ErrorResponse{
				Code:       "NotImplemented",
				Message:    fmt.Sprintf("Invalid Content-Length %d for file uploads to Google Cloud Storage.", fileSize),
				Key:        objectName,
				BucketName: bucketName,
			}
		}
		// Do not compute MD5 for Google Cloud Storage. Uploads upto 5GiB in size.
		n, err := c.putNoChecksum(bucketName, objectName, fileData, fileSize, contentType)
		return n, err
	}

	// NOTE: S3 doesn't allow anonymous multipart requests.
	if isAmazonEndpoint(c.endpointURL) && c.anonymous {
		if fileSize <= -1 || fileSize > int64(maxSinglePutObjectSize) {
			return 0, ErrorResponse{
				Code:       "NotImplemented",
				Message:    fmt.Sprintf("For anonymous requests Content-Length cannot be %d.", fileSize),
				Key:        objectName,
				BucketName: bucketName,
			}
		}
		// Do not compute MD5 for anonymous requests to Amazon S3. Uploads upto 5GiB in size.
		n, err := c.putAnonymous(bucketName, objectName, fileData, fileSize, contentType)
		return n, err
	}

	// Large file upload is initiated for uploads for input data size
	// if its greater than 5MiB or data size is negative.
	if fileSize >= minimumPartSize || fileSize < 0 {
		n, err := c.fputLargeObject(bucketName, objectName, fileData, fileSize, contentType)
		return n, err
	}
	n, err := c.putSmallObject(bucketName, objectName, fileData, fileSize, contentType)
	return n, err
}

// computeHash - calculates MD5 and Sha256 for an input read Seeker.
func (c Client) computeHash(reader io.ReadSeeker) (md5Sum, sha256Sum []byte, size int64, err error) {
	// MD5 and Sha256 hasher.
	var hashMD5, hashSha256 hash.Hash
	// MD5 and Sha256 hasher.
	hashMD5 = md5.New()
	hashWriter := io.MultiWriter(hashMD5)
	if c.signature.isV4() {
		hashSha256 = sha256.New()
		hashWriter = io.MultiWriter(hashMD5, hashSha256)
	}

	size, err = io.Copy(hashWriter, reader)
	if err != nil {
		return nil, nil, 0, err
	}

	// Seek back reader to the beginning location.
	if _, err := reader.Seek(0, 0); err != nil {
		return nil, nil, 0, err
	}

	// Finalize md5shum and sha256 sum.
	md5Sum = hashMD5.Sum(nil)
	if c.signature.isV4() {
		sha256Sum = hashSha256.Sum(nil)
	}
	return md5Sum, sha256Sum, size, nil
}

func (c Client) fputLargeObject(bucketName, objectName string, fileData *os.File, fileSize int64, contentType string) (int64, error) {
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
		totalUploadedSize += partInfo.Size
		// Choose the maximum part size.
		if partInfo.Size >= prevMaxPartSize {
			prevMaxPartSize = partInfo.Size
		}
	}

	// Calculate the optimal part size for a given file size.
	partSize := optimalPartSize(fileSize)
	// If prevMaxPartSize is set use that.
	if prevMaxPartSize != 0 {
		partSize = prevMaxPartSize
	}

	// Part number always starts with '1'.
	partNumber := 1

	// Loop through until EOF.
	for totalUploadedSize < fileSize {
		// Get a section reader on a particular offset.
		sectionReader := io.NewSectionReader(fileData, totalUploadedSize, partSize)

		// Calculates MD5 and Sha256 sum for a section reader.
		md5Sum, sha256Sum, size, err := c.computeHash(sectionReader)
		if err != nil {
			return 0, err
		}

		// Save all the part metadata.
		partMdata := partMetadata{
			ReadCloser: ioutil.NopCloser(sectionReader),
			Size:       size,
			MD5Sum:     md5Sum,
			Sha256Sum:  sha256Sum,
			Number:     partNumber, // Part number to be uploaded.
		}

		// If part number already uploaded, move to the next one.
		if isPartUploaded(objectPart{
			ETag:       hex.EncodeToString(partMdata.MD5Sum),
			PartNumber: partMdata.Number,
		}, partsInfo) {
			// Close the read closer.
			partMdata.ReadCloser.Close()
			continue
		}

		// Upload the part.
		objPart, err := c.uploadPart(bucketName, objectName, uploadID, partMdata)
		if err != nil {
			partMdata.ReadCloser.Close()
			return totalUploadedSize, err
		}

		// Save successfully uploaded size.
		totalUploadedSize += partMdata.Size

		// Save successfully uploaded part metadata.
		partsInfo[partMdata.Number] = objPart

		// Increment to next part number.
		partNumber++
	}

	// if totalUploadedSize is different than the file 'size'. Do not complete the request throw an error.
	if totalUploadedSize != fileSize {
		return totalUploadedSize, ErrUnexpectedEOF(totalUploadedSize, fileSize, bucketName, objectName)
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
