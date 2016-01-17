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
	"hash"
	"io"
	"os"
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

// getUploadID - fetch upload id if already present for an object name
// or initiate a new request to fetch a new upload id.
func (c Client) getUploadID(bucketName, objectName, contentType string) (uploadID string, isNew bool, err error) {
	// Input validation.
	if err := isValidBucketName(bucketName); err != nil {
		return "", false, err
	}
	if err := isValidObjectName(objectName); err != nil {
		return "", false, err
	}

	// Set content Type to default if empty string.
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	// Find upload id for previous upload for an object.
	uploadID, err = c.findUploadID(bucketName, objectName)
	if err != nil {
		return "", false, err
	}
	if uploadID == "" {
		// Initiate multipart upload for an object.
		initMultipartUploadResult, err := c.initiateMultipartUpload(bucketName, objectName, contentType)
		if err != nil {
			return "", false, err
		}
		// Save the new upload id.
		uploadID = initMultipartUploadResult.UploadID
		// Indicate that this is a new upload id.
		isNew = true
	}
	return uploadID, isNew, nil
}

// computeHash - Calculates MD5 and SHA256 for an input read Seeker.
func (c Client) computeHash(reader io.ReadSeeker) (md5Sum, sha256Sum []byte, size int64, err error) {
	// MD5 and SHA256 hasher.
	var hashMD5, hashSHA256 hash.Hash
	// MD5 and SHA256 hasher.
	hashMD5 = md5.New()
	hashWriter := io.MultiWriter(hashMD5)
	if c.signature.isV4() {
		hashSHA256 = sha256.New()
		hashWriter = io.MultiWriter(hashMD5, hashSHA256)
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
		sha256Sum = hashSHA256.Sum(nil)
	}
	return md5Sum, sha256Sum, size, nil
}

// Fetch all parts info, including total uploaded size, maximum part
// size and max part number.
func (c Client) getPartsInfo(bucketName, objectName, uploadID string) (prtsInfo map[int]objectPart, totalSize int64, maxPrtSize int64, maxPrtNumber int, err error) {
	// Fetch previously upload parts.
	prtsInfo, err = c.listObjectParts(bucketName, objectName, uploadID)
	if err != nil {
		return nil, 0, 0, 0, err
	}
	// Peek through all the parts and calculate totalSize, maximum
	// part size and last part number.
	for _, prtInfo := range prtsInfo {
		// Save previously uploaded size.
		totalSize += prtInfo.Size
		// Choose the maximum part size.
		if prtInfo.Size >= maxPrtSize {
			maxPrtSize = prtInfo.Size
		}
		// Choose the maximum part number.
		if maxPrtNumber < prtInfo.PartNumber {
			maxPrtNumber = prtInfo.PartNumber
		}
	}
	return prtsInfo, totalSize, maxPrtSize, maxPrtNumber, nil
}
