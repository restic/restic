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
	"io"
)

// part - message structure for results from the MultiPart
type part struct {
	MD5Sum     []byte
	ReadSeeker io.ReadSeeker
	Err        error
	Len        int64
	Num        int // part number
}

// skipPart - skipping uploaded parts
type skipPart struct {
	md5sum     []byte
	partNumber int
}

// chopper reads from io.Reader, partitions the data into chunks of given chunksize, and sends
// each chunk as io.ReadSeeker to the caller over a channel
//
// This method runs until an EOF or error occurs. If an error occurs,
// the method sends the error over the channel and returns.
// Before returning, the channel is always closed.
//
// additionally this function also skips list of parts if provided
func chopper(reader io.Reader, chunkSize int64, skipParts []skipPart) <-chan part {
	ch := make(chan part, 3)
	go chopperInRoutine(reader, chunkSize, skipParts, ch)
	return ch
}

func chopperInRoutine(reader io.Reader, chunkSize int64, skipParts []skipPart, ch chan part) {
	defer close(ch)
	p := make([]byte, chunkSize)
	n, err := io.ReadFull(reader, p)
	if err == io.EOF || err == io.ErrUnexpectedEOF { // short read, only single part return
		m := md5.Sum(p[0:n])
		ch <- part{
			MD5Sum:     m[:],
			ReadSeeker: bytes.NewReader(p[0:n]),
			Err:        nil,
			Len:        int64(n),
			Num:        1,
		}
		return
	}
	// catastrophic error send error and return
	if err != nil {
		ch <- part{
			ReadSeeker: nil,
			Err:        err,
			Num:        0,
		}
		return
	}
	// send the first part
	var num = 1
	md5SumBytes := md5.Sum(p)
	sp := skipPart{
		partNumber: num,
		md5sum:     md5SumBytes[:],
	}
	if !isPartNumberUploaded(sp, skipParts) {
		ch <- part{
			MD5Sum:     md5SumBytes[:],
			ReadSeeker: bytes.NewReader(p),
			Err:        nil,
			Len:        int64(n),
			Num:        num,
		}
	}
	for err == nil {
		var n int
		p := make([]byte, chunkSize)
		n, err = io.ReadFull(reader, p)
		if err != nil {
			if err != io.EOF && err != io.ErrUnexpectedEOF { // catastrophic error
				ch <- part{
					ReadSeeker: nil,
					Err:        err,
					Num:        0,
				}
				return
			}
		}
		num++
		md5SumBytes := md5.Sum(p[0:n])
		sp := skipPart{
			partNumber: num,
			md5sum:     md5SumBytes[:],
		}
		if isPartNumberUploaded(sp, skipParts) {
			continue
		}
		ch <- part{
			MD5Sum:     md5SumBytes[:],
			ReadSeeker: bytes.NewReader(p[0:n]),
			Err:        nil,
			Len:        int64(n),
			Num:        num,
		}

	}
}

// to verify if partNumber is part of the skip part list
func isPartNumberUploaded(part skipPart, skipParts []skipPart) bool {
	for _, p := range skipParts {
		if p.partNumber == part.partNumber && bytes.Equal(p.md5sum, part.md5sum) {
			return true
		}
	}
	return false
}
