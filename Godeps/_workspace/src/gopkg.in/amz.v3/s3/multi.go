package s3

import (
	"bytes"
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"io"
	"net/http"
	"sort"
	"strconv"
)

// Multi represents an unfinished multipart upload.
//
// Multipart uploads allow sending big objects in smaller chunks.
// After all parts have been sent, the upload must be explicitly
// completed by calling Complete with the list of parts.
//
// See http://goo.gl/vJfTG for an overview of multipart uploads.
type Multi struct {
	Bucket   *Bucket
	Key      string
	UploadId string
}

// That's the default. Here just for testing.
var listMultiMax = 1000

type listMultiResp struct {
	NextKeyMarker      string
	NextUploadIdMarker string
	IsTruncated        bool
	Upload             []Multi
	CommonPrefixes     []string `xml:"CommonPrefixes>Prefix"`
}

// ListMulti returns the list of unfinished multipart uploads in b.
//
// The prefix parameter limits the response to keys that begin with the
// specified prefix. You can use prefixes to separate a bucket into different
// groupings of keys (to get the feeling of folders, for example).
//
// The delim parameter causes the response to group all of the keys that
// share a common prefix up to the next delimiter in a single entry within
// the CommonPrefixes field. You can use delimiters to separate a bucket
// into different groupings of keys, similar to how folders would work.
//
// See http://goo.gl/ePioY for details.
func (b *Bucket) ListMulti(prefix, delim string) (multis []*Multi, prefixes []string, err error) {

	req, err := http.NewRequest("GET", b.Region.ResolveS3BucketEndpoint(b.Name), nil)
	if err != nil {
		return nil, nil, err
	}

	query := req.URL.Query()
	query.Add("uploads", "")
	query.Add("max-uploads", strconv.FormatInt(int64(listMultiMax), 10))
	query.Add("prefix", prefix)
	query.Add("delimiter", delim)
	req.URL.RawQuery = query.Encode()

	addAmazonDateHeader(req.Header)

	// We need to resign every iteration because we're changing variables.
	if err := b.S3.Sign(req, b.Auth); err != nil {
		return nil, nil, err
	}

	for attempt := attempts.Start(); attempt.Next(); {

		resp, err := requestRetryLoop(req, attempts)
		if err == nil {
			if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
				err = buildError(resp)
			}
		}
		if err != nil {
			if shouldRetry(err) && attempt.HasNext() {
				continue
			}
			return nil, nil, err
		}

		var multiResp listMultiResp
		if err := xml.NewDecoder(resp.Body).Decode(&multiResp); err != nil {
			return nil, nil, err
		}
		resp.Body.Close()

		for i := range multiResp.Upload {
			multi := &multiResp.Upload[i]
			multi.Bucket = b
			multis = append(multis, multi)
		}
		prefixes = append(prefixes, multiResp.CommonPrefixes...)
		if !multiResp.IsTruncated {
			return multis, prefixes, nil
		}

		query := req.URL.Query()
		query.Set("key-marker", multiResp.NextKeyMarker)
		query.Set("upload-id-marker", multiResp.NextUploadIdMarker)
		req.URL.RawQuery = query.Encode()

		// Last request worked; restart our counter.
		attempt = attempts.Start()
	}

	panic("unreachable")
}

// Multi returns a multipart upload handler for the provided key
// inside b. If a multipart upload exists for key, it is returned,
// otherwise a new multipart upload is initiated with contType and perm.
func (b *Bucket) Multi(key, contType string, perm ACL) (*Multi, error) {
	multis, _, err := b.ListMulti(key, "")
	if err != nil && !hasCode(err, "NoSuchUpload") {
		return nil, err
	}
	for _, m := range multis {
		if m.Key == key {
			return m, nil
		}
	}
	return b.InitMulti(key, contType, perm)
}

// InitMulti initializes a new multipart upload at the provided
// key inside b and returns a value for manipulating it.
//
// See http://goo.gl/XP8kL for details.
func (b *Bucket) InitMulti(key string, contType string, perm ACL) (*Multi, error) {

	req, err := http.NewRequest("POST", b.Region.ResolveS3BucketEndpoint(b.Name), nil)
	if err != nil {
		return nil, err
	}
	req.URL.Path += key

	query := req.URL.Query()
	query.Add("uploads", "")
	req.URL.RawQuery = query.Encode()

	req.Header.Add("Content-Type", contType)
	req.Header.Add("Content-Length", "0")
	req.Header.Add("x-amz-acl", string(perm))
	addAmazonDateHeader(req.Header)

	if err := b.S3.Sign(req, b.Auth); err != nil {
		return nil, err
	}

	resp, err := requestRetryLoop(req, attempts)
	if err == nil {
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
			err = buildError(resp)
		}
	}
	if err != nil {
		return nil, err
	}

	var multiResp struct {
		UploadId string `xml:"UploadId"`
	}
	if err := xml.NewDecoder(resp.Body).Decode(&multiResp); err != nil {
		return nil, err
	}

	return &Multi{Bucket: b, Key: key, UploadId: multiResp.UploadId}, nil
}

// PutPart sends part n of the multipart upload, reading all the content from r.
// Each part, except for the last one, must be at least 5MB in size.
//
// See http://goo.gl/pqZer for details.
func (m *Multi) PutPart(n int, r io.ReadSeeker) (Part, error) {
	partSize, _, md5b64, err := seekerInfo(r)
	if err != nil {
		return Part{}, err
	}
	return m.putPart(n, r, partSize, md5b64)
}

func (m *Multi) putPart(n int, r io.ReadSeeker, partSize int64, md5b64 string) (Part, error) {
	_, err := r.Seek(0, 0)
	if err != nil {
		return Part{}, err
	}

	req, err := http.NewRequest("PUT", m.Bucket.Region.ResolveS3BucketEndpoint(m.Bucket.Name), r)
	if err != nil {
		return Part{}, err
	}
	req.Close = true
	req.URL.Path += m.Key
	req.ContentLength = partSize

	query := req.URL.Query()
	query.Add("uploadId", m.UploadId)
	query.Add("partNumber", strconv.FormatInt(int64(n), 10))
	req.URL.RawQuery = query.Encode()

	req.Header.Add("Content-MD5", md5b64)
	addAmazonDateHeader(req.Header)

	if err := m.Bucket.S3.Sign(req, m.Bucket.Auth); err != nil {
		return Part{}, err
	}
	// Signing may read the request body.
	if _, err := r.Seek(0, 0); err != nil {
		return Part{}, err
	}

	resp, err := requestRetryLoop(req, attempts)
	defer resp.Body.Close()

	if err != nil {
		return Part{}, err
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return Part{}, buildError(resp)
	}

	part := Part{n, resp.Header.Get("ETag"), partSize}
	if part.ETag == "" {
		return Part{}, errors.New("part upload succeeded with no ETag")
	}

	return part, nil
}

func seekerInfo(r io.ReadSeeker) (size int64, md5hex string, md5b64 string, err error) {
	_, err = r.Seek(0, 0)
	if err != nil {
		return 0, "", "", err
	}
	digest := md5.New()
	size, err = io.Copy(digest, r)
	if err != nil {
		return 0, "", "", err
	}
	sum := digest.Sum(nil)
	md5hex = hex.EncodeToString(sum)
	md5b64 = base64.StdEncoding.EncodeToString(sum)
	return size, md5hex, md5b64, nil
}

type Part struct {
	N    int `xml:"PartNumber"`
	ETag string
	Size int64
}

type partSlice []Part

func (s partSlice) Len() int           { return len(s) }
func (s partSlice) Less(i, j int) bool { return s[i].N < s[j].N }
func (s partSlice) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

type listPartsResp struct {
	NextPartNumberMarker string
	IsTruncated          bool
	Part                 []Part
}

// That's the default. Here just for testing.
var listPartsMax = 1000

// ListParts returns the list of previously uploaded parts in m,
// ordered by part number.
//
// See http://goo.gl/ePioY for details.
func (m *Multi) ListParts() ([]Part, error) {

	req, err := http.NewRequest("GET", m.Bucket.Region.ResolveS3BucketEndpoint(m.Bucket.Name), nil)
	if err != nil {
		return nil, err
	}
	req.Close = true
	req.URL.Path += m.Key

	query := req.URL.Query()
	query.Add("uploadId", m.UploadId)
	query.Add("max-parts", strconv.FormatInt(int64(listPartsMax), 10))
	req.URL.RawQuery = query.Encode()

	var parts partSlice
	for attempt := attempts.Start(); attempt.Next(); {

		addAmazonDateHeader(req.Header)

		// We need to resign every iteration because we're changing the URL.
		if err := m.Bucket.S3.Sign(req, m.Bucket.Auth); err != nil {
			return nil, err
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, err
		} else if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
			err = buildError(resp)
		}

		if err != nil {
			if shouldRetry(err) && attempt.HasNext() {
				continue
			}
			return nil, err
		}

		var listResp listPartsResp
		if err := xml.NewDecoder(resp.Body).Decode(&listResp); err != nil {
			return nil, err
		}

		parts = append(parts, listResp.Part...)
		if listResp.IsTruncated == false {
			break
		}

		query.Set("part-number-marker", listResp.NextPartNumberMarker)
		req.URL.RawQuery = query.Encode()

		// Last request worked; restart our counter.
		attempt = attempts.Start()
	}

	sort.Sort(parts)
	return parts, nil
}

type ReaderAtSeeker interface {
	io.ReaderAt
	io.ReadSeeker
}

// PutAll sends all of r via a multipart upload with parts no larger
// than partSize bytes, which must be set to at least 5MB.
// Parts previously uploaded are either reused if their checksum
// and size match the new part, or otherwise overwritten with the
// new content.
// PutAll returns all the parts of m (reused or not).
func (m *Multi) PutAll(r ReaderAtSeeker, partSize int64) ([]Part, error) {
	old, err := m.ListParts()
	if err != nil && !hasCode(err, "NoSuchUpload") {
		return nil, err
	}

	reuse := 0   // Index of next old part to consider reusing.
	current := 1 // Part number of latest good part handled.
	totalSize, err := r.Seek(0, 2)
	if err != nil {
		return nil, err
	}
	first := true // Must send at least one empty part if the file is empty.
	var result []Part
NextSection:
	for offset := int64(0); offset < totalSize || first; offset += partSize {
		first = false
		if offset+partSize > totalSize {
			partSize = totalSize - offset
		}
		section := io.NewSectionReader(r, offset, partSize)
		_, md5hex, md5b64, err := seekerInfo(section)
		if err != nil {
			return nil, err
		}
		for reuse < len(old) && old[reuse].N <= current {
			// Looks like this part was already sent.
			part := &old[reuse]
			etag := `"` + md5hex + `"`
			if part.N == current && part.Size == partSize && part.ETag == etag {
				// Checksum matches. Reuse the old part.
				result = append(result, *part)
				current++
				continue NextSection
			}
			reuse++
		}

		// Part wasn't found or doesn't match. Send it.
		part, err := m.putPart(current, section, partSize, md5b64)
		if err != nil {
			return nil, err
		}
		result = append(result, part)
		current++
	}
	return result, nil
}

type completeUpload struct {
	XMLName xml.Name      `xml:"CompleteMultipartUpload"`
	Parts   completeParts `xml:"Part"`
}

type completePart struct {
	PartNumber int
	ETag       string
}

type completeParts []completePart

func (p completeParts) Len() int           { return len(p) }
func (p completeParts) Less(i, j int) bool { return p[i].PartNumber < p[j].PartNumber }
func (p completeParts) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }

// Complete assembles the given previously uploaded parts into the
// final object. This operation may take several minutes.
//
// See http://goo.gl/2Z7Tw for details.
func (m *Multi) Complete(parts []Part) error {

	var c completeUpload
	for _, p := range parts {
		c.Parts = append(c.Parts, completePart{p.N, p.ETag})
	}
	sort.Sort(c.Parts)

	data, err := xml.Marshal(&c)
	if err != nil {
		return err
	}
	body := bytes.NewReader(data)

	req, err := http.NewRequest(
		"POST",
		m.Bucket.Region.ResolveS3BucketEndpoint(m.Bucket.Name),
		body,
	)
	if err != nil {
		return err
	}
	req.Close = true
	req.ContentLength = int64(len(data))
	req.URL.Path += m.Key

	query := req.URL.Query()
	query.Add("uploadId", m.UploadId)
	req.URL.RawQuery = query.Encode()

	addAmazonDateHeader(req.Header)

	if err := m.Bucket.S3.Sign(req, m.Bucket.Auth); err != nil {
		return err
	}
	// Signing may read the request body.
	if _, err := body.Seek(0, 0); err != nil {
		return err
	}

	resp, err := requestRetryLoop(req, attempts)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return buildError(resp)
	}

	return nil
}

// Abort deletes an unifinished multipart upload and any previously
// uploaded parts for it.
//
// After a multipart upload is aborted, no additional parts can be
// uploaded using it. However, if any part uploads are currently in
// progress, those part uploads might or might not succeed. As a result,
// it might be necessary to abort a given multipart upload multiple
// times in order to completely free all storage consumed by all parts.
//
// NOTE: If the described scenario happens to you, please report back to
// the goamz authors with details. In the future such retrying should be
// handled internally, but it's not clear what happens precisely (Is an
// error returned? Is the issue completely undetectable?).
//
// See http://goo.gl/dnyJw for details.
func (m *Multi) Abort() error {

	req, err := http.NewRequest("DELETE", m.Bucket.Region.ResolveS3BucketEndpoint(m.Bucket.Name), nil)
	if err != nil {
		return nil
	}
	req.URL.Path += m.Key

	query := req.URL.Query()
	query.Add("uploadId", m.UploadId)
	req.URL.RawQuery = query.Encode()

	addAmazonDateHeader(req.Header)

	if err := m.Bucket.S3.Sign(req, m.Bucket.Auth); err != nil {
		return err
	}
	_, err = requestRetryLoop(req, attempts)
	return err
}
