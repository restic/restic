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
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

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

// getHashedPayload get the hexadecimal value of the SHA256 hash of the request payload
func (r *request) getHashedPayload() string {
	hash := func() string {
		switch {
		case r.expires != 0:
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
func (r *request) getStringToSignV4(canonicalRequest string, t time.Time) string {
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

func (r *request) PostPresignSignatureV4(policyBase64 string, t time.Time) string {
	signingkey := getSigningKey(r.config.SecretAccessKey, r.config.Region, t)
	signature := getSignature(signingkey, policyBase64)
	return signature
}

// SignV4 the request before Do(), in accordance with - http://docs.aws.amazon.com/AmazonS3/latest/API/sig-v4-authenticating-requests.html
func (r *request) SignV4() {
	query := r.req.URL.Query()
	if r.expires != 0 {
		query.Set("X-Amz-Algorithm", authHeader)
	}
	t := time.Now().UTC()
	// Add date if not present
	if r.expires != 0 {
		query.Set("X-Amz-Date", t.Format(iso8601DateFormat))
		query.Set("X-Amz-Expires", strconv.FormatInt(r.expires, 10))
	} else {
		r.Set("X-Amz-Date", t.Format(iso8601DateFormat))
	}

	hashedPayload := r.getHashedPayload()
	signedHeaders := r.getSignedHeaders()
	if r.expires != 0 {
		query.Set("X-Amz-SignedHeaders", signedHeaders)
	}
	credential := getCredential(r.config.AccessKeyID, r.config.Region, t)
	if r.expires != 0 {
		query.Set("X-Amz-Credential", credential)
		r.req.URL.RawQuery = query.Encode()
	}
	canonicalRequest := r.getCanonicalRequest(hashedPayload)
	stringToSign := r.getStringToSignV4(canonicalRequest, t)
	signingKey := getSigningKey(r.config.SecretAccessKey, r.config.Region, t)
	signature := getSignature(signingKey, stringToSign)

	if r.expires != 0 {
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
