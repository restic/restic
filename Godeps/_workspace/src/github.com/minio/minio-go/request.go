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
	"encoding/hex"
	"errors"
	"io"
	"io/ioutil"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

// operation - rest operation
type operation struct {
	HTTPServer string
	HTTPMethod string
	HTTPPath   string
}

// request - a http request
type request struct {
	req     *http.Request
	config  *Config
	body    io.ReadSeeker
	expires string
}

const (
	authHeader        = "AWS4-HMAC-SHA256"
	iso8601DateFormat = "20060102T150405Z"
	yyyymmdd          = "20060102"
)

///
/// Excerpts from @lsegal - https://github.com/aws/aws-sdk-js/issues/659#issuecomment-120477258
///
///  User-Agent:
///
///      This is ignored from signing because signing this causes problems with generating pre-signed URLs
///      (that are executed by other agents) or when customers pass requests through proxies, which may
///      modify the user-agent.
///
///  Content-Length:
///
///      This is ignored from signing because generating a pre-signed URL should not provide a content-length
///      constraint, specifically when vending a S3 pre-signed PUT URL. The corollary to this is that when
///      sending regular requests (non-pre-signed), the signature contains a checksum of the body, which
///      implicitly validates the payload length (since changing the number of bytes would change the checksum)
///      and therefore this header is not valuable in the signature.
///
///  Content-Type:
///
///      Signing this header causes quite a number of problems in browser environments, where browsers
///      like to modify and normalize the content-type header in different ways. There is more information
///      on this in https://github.com/aws/aws-sdk-js/issues/244. Avoiding this field simplifies logic
///      and reduces the possibility of future bugs
///
///  Authorization:
///
///      Is skipped for obvious reasons
///
var ignoredHeaders = map[string]bool{
	"Authorization":  true,
	"Content-Type":   true,
	"Content-Length": true,
	"User-Agent":     true,
}

// getURLEncodedPath encode the strings from UTF-8 byte representations to HTML hex escape sequences
//
// This is necessary since regular url.Parse() and url.Encode() functions do not support UTF-8
// non english characters cannot be parsed due to the nature in which url.Encode() is written
//
// This function on the other hand is a direct replacement for url.Encode() technique to support
// pretty much every UTF-8 character.
func getURLEncodedPath(pathName string) string {
	// if object matches reserved string, no need to encode them
	reservedNames := regexp.MustCompile("^[a-zA-Z0-9-_.~/]+$")
	if reservedNames.MatchString(pathName) {
		return pathName
	}
	var encodedPathname string
	for _, s := range pathName {
		if 'A' <= s && s <= 'Z' || 'a' <= s && s <= 'z' || '0' <= s && s <= '9' { // §2.3 Unreserved characters (mark)
			encodedPathname = encodedPathname + string(s)
			continue
		}
		switch s {
		case '-', '_', '.', '~', '/': // §2.3 Unreserved characters (mark)
			encodedPathname = encodedPathname + string(s)
			continue
		default:
			len := utf8.RuneLen(s)
			if len < 0 {
				// if utf8 cannot convert return the same string as is
				return pathName
			}
			u := make([]byte, len)
			utf8.EncodeRune(u, s)
			for _, r := range u {
				hex := hex.EncodeToString([]byte{r})
				encodedPathname = encodedPathname + "%" + strings.ToUpper(hex)
			}
		}
	}
	return encodedPathname
}

func path2BucketAndObject(path string) (bucketName, objectName string) {
	pathSplits := strings.SplitN(path, "?", 2)
	splits := strings.SplitN(pathSplits[0], separator, 3)
	switch len(splits) {
	case 0, 1:
		bucketName = ""
		objectName = ""
	case 2:
		bucketName = splits[1]
		objectName = ""
	case 3:
		bucketName = splits[1]
		objectName = splits[2]
	}
	return bucketName, objectName
}

// path2Object gives objectName from URL path
func path2Object(path string) (objectName string) {
	_, objectName = path2BucketAndObject(path)
	return
}

// path2Bucket gives bucketName from URL path
func path2Bucket(path string) (bucketName string) {
	bucketName, _ = path2BucketAndObject(path)
	return
}

// path2Query gives query part from URL path
func path2Query(path string) (query string) {
	pathSplits := strings.SplitN(path, "?", 2)
	if len(pathSplits) > 1 {
		query = pathSplits[1]
	}
	return
}

func (op *operation) getRequestURL(config Config) (url string) {
	// parse URL for the combination of HTTPServer + HTTPPath
	url = op.HTTPServer + separator
	if !config.isVirtualStyle {
		url += path2Bucket(op.HTTPPath)
	}
	objectName := getURLEncodedPath(path2Object(op.HTTPPath))
	queryPath := path2Query(op.HTTPPath)
	if objectName == "" && queryPath != "" {
		url += "?" + queryPath
		return
	}
	if objectName != "" && queryPath == "" {
		if strings.HasSuffix(url, separator) {
			url += objectName
		} else {
			url += separator + objectName
		}
		return
	}
	if objectName != "" && queryPath != "" {
		if strings.HasSuffix(url, separator) {
			url += objectName + "?" + queryPath
		} else {
			url += separator + objectName + "?" + queryPath
		}
	}
	return
}

func newPresignedRequest(op *operation, config *Config, expires string) (*request, error) {
	// if no method default to POST
	method := op.HTTPMethod
	if method == "" {
		method = "POST"
	}

	u := op.getRequestURL(*config)

	// get a new HTTP request, for the requested method
	req, err := http.NewRequest(method, u, nil)
	if err != nil {
		return nil, err
	}

	// set UserAgent
	req.Header.Set("User-Agent", config.userAgent)

	// set Accept header for response encoding style, if available
	if config.AcceptType != "" {
		req.Header.Set("Accept", config.AcceptType)
	}

	// save for subsequent use
	r := new(request)
	r.config = config
	r.expires = expires
	r.req = req
	r.body = nil

	return r, nil
}

// newUnauthenticatedRequest - instantiate a new unauthenticated request
func newUnauthenticatedRequest(op *operation, config *Config, body io.Reader) (*request, error) {
	// if no method default to POST
	method := op.HTTPMethod
	if method == "" {
		method = "POST"
	}

	u := op.getRequestURL(*config)

	// get a new HTTP request, for the requested method
	req, err := http.NewRequest(method, u, nil)
	if err != nil {
		return nil, err
	}

	// set UserAgent
	req.Header.Set("User-Agent", config.userAgent)

	// set Accept header for response encoding style, if available
	if config.AcceptType != "" {
		req.Header.Set("Accept", config.AcceptType)
	}

	// add body
	switch {
	case body == nil:
		req.Body = nil
	default:
		req.Body = ioutil.NopCloser(body)
	}

	// save for subsequent use
	r := new(request)
	r.req = req
	r.config = config

	return r, nil
}

// newRequest - instantiate a new request
func newRequest(op *operation, config *Config, body io.ReadSeeker) (*request, error) {
	// if no method default to POST
	method := op.HTTPMethod
	if method == "" {
		method = "POST"
	}

	u := op.getRequestURL(*config)

	// get a new HTTP request, for the requested method
	req, err := http.NewRequest(method, u, nil)
	if err != nil {
		return nil, err
	}

	// set UserAgent
	req.Header.Set("User-Agent", config.userAgent)

	// set Accept header for response encoding style, if available
	if config.AcceptType != "" {
		req.Header.Set("Accept", config.AcceptType)
	}

	// add body
	switch {
	case body == nil:
		req.Body = nil
	default:
		req.Body = ioutil.NopCloser(body)
	}

	// save for subsequent use
	r := new(request)
	r.config = config
	r.req = req
	r.body = body

	return r, nil
}

// Do - start the request
func (r *request) Do() (resp *http.Response, err error) {
	if r.config.AccessKeyID != "" && r.config.SecretAccessKey != "" {
		r.SignV4()
	}
	transport := http.DefaultTransport
	if r.config.Transport != nil {
		transport = r.config.Transport
	}
	// do not use http.Client{}, while it may seem intuitive but the problem seems to be
	// that http.Client{} internally follows redirects and there is no easier way to disable
	// it from outside using a configuration parameter -
	//     this auto redirect causes complications in verifying subsequent errors
	//
	// The best is to use RoundTrip() directly, so the request comes back to the caller where
	// we are going to handle such replies. And indeed that is the right thing to do here.
	//
	return transport.RoundTrip(r.req)
}

// Set - set additional headers if any
func (r *request) Set(key, value string) {
	r.req.Header.Set(key, value)
}

// Get - get header values
func (r *request) Get(key string) string {
	return r.req.Header.Get(key)
}

// getHashedPayload get the hexadecimal value of the SHA256 hash of the request payload
func (r *request) getHashedPayload() string {
	hash := func() string {
		switch {
		case r.expires != "":
			return "UNSIGNED-PAYLOAD"
		case r.body == nil:
			return hex.EncodeToString(sum256([]byte{}))
		default:
			sum256Bytes, _ := sum256Reader(r.body)
			return hex.EncodeToString(sum256Bytes)
		}
	}
	hashedPayload := hash()
	if hashedPayload != "UNSIGNED-PAYLOAD" {
		r.req.Header.Set("X-Amz-Content-Sha256", hashedPayload)
	}
	return hashedPayload
}

// getCanonicalHeaders generate a list of request headers with their values
func (r *request) getCanonicalHeaders() string {
	var headers []string
	vals := make(map[string][]string)
	for k, vv := range r.req.Header {
		if _, ok := ignoredHeaders[http.CanonicalHeaderKey(k)]; ok {
			continue // ignored header
		}
		headers = append(headers, strings.ToLower(k))
		vals[strings.ToLower(k)] = vv
	}
	headers = append(headers, "host")
	sort.Strings(headers)

	var buf bytes.Buffer
	for _, k := range headers {
		buf.WriteString(k)
		buf.WriteByte(':')
		switch {
		case k == "host":
			buf.WriteString(r.req.URL.Host)
			fallthrough
		default:
			for idx, v := range vals[k] {
				if idx > 0 {
					buf.WriteByte(',')
				}
				buf.WriteString(v)
			}
			buf.WriteByte('\n')
		}
	}
	return buf.String()
}

// getSignedHeaders generate a string i.e alphabetically sorted, semicolon-separated list of lowercase request header names
func (r *request) getSignedHeaders() string {
	var headers []string
	for k := range r.req.Header {
		if _, ok := ignoredHeaders[http.CanonicalHeaderKey(k)]; ok {
			continue // ignored header
		}
		headers = append(headers, strings.ToLower(k))
	}
	headers = append(headers, "host")
	sort.Strings(headers)
	return strings.Join(headers, ";")
}

// getCanonicalRequest generate a canonical request of style
//
// canonicalRequest =
//  <HTTPMethod>\n
//  <CanonicalURI>\n
//  <CanonicalQueryString>\n
//  <CanonicalHeaders>\n
//  <SignedHeaders>\n
//  <HashedPayload>
//
func (r *request) getCanonicalRequest(hashedPayload string) string {
	r.req.URL.RawQuery = strings.Replace(r.req.URL.Query().Encode(), "+", "%20", -1)
	canonicalRequest := strings.Join([]string{
		r.req.Method,
		getURLEncodedPath(r.req.URL.Path),
		r.req.URL.RawQuery,
		r.getCanonicalHeaders(),
		r.getSignedHeaders(),
		hashedPayload,
	}, "\n")
	return canonicalRequest
}

// getStringToSign a string based on selected query values
func (r *request) getStringToSign(canonicalRequest string, t time.Time) string {
	stringToSign := authHeader + "\n" + t.Format(iso8601DateFormat) + "\n"
	stringToSign = stringToSign + getScope(r.config.Region, t) + "\n"
	stringToSign = stringToSign + hex.EncodeToString(sum256([]byte(canonicalRequest)))
	return stringToSign
}

// Presign the request, in accordance with - http://docs.aws.amazon.com/AmazonS3/latest/API/sigv4-query-string-auth.html
func (r *request) PreSignV4() (string, error) {
	if r.config.AccessKeyID == "" && r.config.SecretAccessKey == "" {
		return "", errors.New("presign requires accesskey and secretkey")
	}
	r.SignV4()
	return r.req.URL.String(), nil
}

func (r *request) PostPresignSignature(policyBase64 string, t time.Time) string {
	signingkey := getSigningKey(r.config.SecretAccessKey, r.config.Region, t)
	signature := getSignature(signingkey, policyBase64)
	return signature
}

// SignV4 the request before Do(), in accordance with - http://docs.aws.amazon.com/AmazonS3/latest/API/sig-v4-authenticating-requests.html
func (r *request) SignV4() {
	query := r.req.URL.Query()
	if r.expires != "" {
		query.Set("X-Amz-Algorithm", authHeader)
	}
	t := time.Now().UTC()
	// Add date if not present
	if r.expires != "" {
		query.Set("X-Amz-Date", t.Format(iso8601DateFormat))
		query.Set("X-Amz-Expires", r.expires)
	} else {
		r.Set("X-Amz-Date", t.Format(iso8601DateFormat))
	}

	hashedPayload := r.getHashedPayload()
	signedHeaders := r.getSignedHeaders()
	if r.expires != "" {
		query.Set("X-Amz-SignedHeaders", signedHeaders)
	}
	credential := getCredential(r.config.AccessKeyID, r.config.Region, t)
	if r.expires != "" {
		query.Set("X-Amz-Credential", credential)
		r.req.URL.RawQuery = query.Encode()
	}
	canonicalRequest := r.getCanonicalRequest(hashedPayload)
	stringToSign := r.getStringToSign(canonicalRequest, t)
	signingKey := getSigningKey(r.config.SecretAccessKey, r.config.Region, t)
	signature := getSignature(signingKey, stringToSign)

	if r.expires != "" {
		r.req.URL.RawQuery += "&X-Amz-Signature=" + signature
	} else {
		// final Authorization header
		parts := []string{
			authHeader + " Credential=" + credential,
			"SignedHeaders=" + signedHeaders,
			"Signature=" + signature,
		}
		auth := strings.Join(parts, ", ")
		r.Set("Authorization", auth)
	}
}
