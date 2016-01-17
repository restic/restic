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

/// Multipart upload defaults.

// minimumPartSize - minimum part size 5MiB per object after which
// putObject behaves internally as multipart.
const minimumPartSize = 1024 * 1024 * 5

// maxParts - maximum parts for a single multipart session.
const maxParts = 10000

// maxPartSize - maximum part size 5GiB for a single multipart upload
// operation.
const maxPartSize = 1024 * 1024 * 1024 * 5

// maxSinglePutObjectSize - maximum size 5GiB of object per PUT
// operation.
const maxSinglePutObjectSize = 1024 * 1024 * 1024 * 5

// maxMultipartPutObjectSize - maximum size 5TiB of object for
// Multipart operation.
const maxMultipartPutObjectSize = 1024 * 1024 * 1024 * 1024 * 5

// optimalReadAtBufferSize - optimal buffer 5MiB used for reading
// through ReadAt operation.
const optimalReadAtBufferSize = 1024 * 1024 * 5
