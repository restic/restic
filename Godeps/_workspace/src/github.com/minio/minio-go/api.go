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
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"runtime"
	"strings"
	"time"
)

// Client implements Amazon S3 compatible methods.
type Client struct {
	///  Standard options.
	accessKeyID     string        // AccessKeyID required for authorized requests.
	secretAccessKey string        // SecretAccessKey required for authorized requests.
	signature       SignatureType // Choose a signature type if necessary.
	anonymous       bool          // Set to 'true' if Client has no access and secret keys.

	// User supplied.
	appInfo struct {
		appName    string
		appVersion string
	}
	endpointURL *url.URL

	// Needs allocation.
	httpClient     *http.Client
	bucketLocCache *bucketLocationCache

	// Advanced functionality
	isTraceEnabled bool
	traceOutput    io.Writer
}

// Global constants.
const (
	libraryName    = "minio-go"
	libraryVersion = "0.2.5"
)

// User Agent should always following the below style.
// Please open an issue to discuss any new changes here.
//
//       Minio (OS; ARCH) LIB/VER APP/VER
const (
	libraryUserAgentPrefix = "Minio (" + runtime.GOOS + "; " + runtime.GOARCH + ") "
	libraryUserAgent       = libraryUserAgentPrefix + libraryName + "/" + libraryVersion
)

// NewV2 - instantiate minio client with Amazon S3 signature version '2' compatiblity.
func NewV2(endpoint string, accessKeyID, secretAccessKey string, insecure bool) (CloudStorageClient, error) {
	clnt, err := privateNew(endpoint, accessKeyID, secretAccessKey, insecure)
	if err != nil {
		return nil, err
	}
	// Set to use signature version '2'.
	clnt.signature = SignatureV2
	return clnt, nil
}

// NewV4 - instantiate minio client with Amazon S3 signature version '4' compatibility.
func NewV4(endpoint string, accessKeyID, secretAccessKey string, insecure bool) (CloudStorageClient, error) {
	clnt, err := privateNew(endpoint, accessKeyID, secretAccessKey, insecure)
	if err != nil {
		return nil, err
	}
	// Set to use signature version '4'.
	clnt.signature = SignatureV4
	return clnt, nil
}

// New - instantiate minio client Client, adds automatic verification of signature.
func New(endpoint string, accessKeyID, secretAccessKey string, insecure bool) (CloudStorageClient, error) {
	clnt, err := privateNew(endpoint, accessKeyID, secretAccessKey, insecure)
	if err != nil {
		return nil, err
	}
	// Google cloud storage should be set to signature V2, force it if not.
	if isGoogleEndpoint(clnt.endpointURL) {
		clnt.signature = SignatureV2
	}
	// If Amazon S3 set to signature v2.
	if isAmazonEndpoint(clnt.endpointURL) {
		clnt.signature = SignatureV4
	}
	return clnt, nil
}

func privateNew(endpoint, accessKeyID, secretAccessKey string, insecure bool) (*Client, error) {
	// construct endpoint.
	endpointURL, err := getEndpointURL(endpoint, insecure)
	if err != nil {
		return nil, err
	}

	// instantiate new Client.
	clnt := new(Client)
	clnt.accessKeyID = accessKeyID
	clnt.secretAccessKey = secretAccessKey
	if clnt.accessKeyID == "" || clnt.secretAccessKey == "" {
		clnt.anonymous = true
	}

	// Save endpoint URL, user agent for future uses.
	clnt.endpointURL = endpointURL

	// Instantiate http client and bucket location cache.
	clnt.httpClient = &http.Client{}
	clnt.bucketLocCache = newBucketLocationCache()

	// Return.
	return clnt, nil
}

// SetAppInfo - add application details to user agent.
func (c *Client) SetAppInfo(appName string, appVersion string) {
	// if app name and version is not set, we do not a new user agent.
	if appName != "" && appVersion != "" {
		c.appInfo = struct {
			appName    string
			appVersion string
		}{}
		c.appInfo.appName = appName
		c.appInfo.appVersion = appVersion
	}
}

// SetCustomTransport - set new custom transport.
func (c *Client) SetCustomTransport(customHTTPTransport http.RoundTripper) {
	// Set this to override default transport ``http.DefaultTransport``.
	//
	// This transport is usually needed for debugging OR to add your own
	// custom TLS certificates on the client transport, for custom CA's and
	// certs which are not part of standard certificate authority follow this
	// example :-
	//
	//   tr := &http.Transport{
	//           TLSClientConfig:    &tls.Config{RootCAs: pool},
	//           DisableCompression: true,
	//   }
	//   api.SetTransport(tr)
	//
	if c.httpClient != nil {
		c.httpClient.Transport = customHTTPTransport
	}
}

// TraceOn - enable HTTP tracing.
func (c *Client) TraceOn(outputStream io.Writer) error {
	// if outputStream is nil then default to os.Stdout.
	if outputStream == nil {
		outputStream = os.Stdout
	}
	// Sets a new output stream.
	c.traceOutput = outputStream

	// Enable tracing.
	c.isTraceEnabled = true
	return nil
}

// TraceOff - disable HTTP tracing.
func (c *Client) TraceOff() {
	// Disable tracing.
	c.isTraceEnabled = false
}

// requestMetadata - is container for all the values to make a request.
type requestMetadata struct {
	// If set newRequest presigns the URL.
	presignURL bool

	// User supplied.
	bucketName   string
	objectName   string
	queryValues  url.Values
	customHeader http.Header
	expires      int64

	// Generated by our internal code.
	contentBody        io.ReadCloser
	contentLength      int64
	contentSha256Bytes []byte
	contentMD5Bytes    []byte
}

// dumpHTTP - dump HTTP request and response.
func (c Client) dumpHTTP(req *http.Request, resp *http.Response) error {
	// Starts http dump.
	_, err := fmt.Fprintln(c.traceOutput, "---------START-HTTP---------")
	if err != nil {
		return err
	}

	// Only display request header.
	reqTrace, err := httputil.DumpRequestOut(req, false)
	if err != nil {
		return err
	}

	// Write request to trace output.
	_, err = fmt.Fprint(c.traceOutput, string(reqTrace))
	if err != nil {
		return err
	}

	// Only display response header.
	respTrace, err := httputil.DumpResponse(resp, false)
	if err != nil {
		return err
	}

	// Write response to trace output.
	_, err = fmt.Fprint(c.traceOutput, strings.TrimSuffix(string(respTrace), "\r\n"))
	if err != nil {
		return err
	}

	// Ends the http dump.
	_, err = fmt.Fprintln(c.traceOutput, "---------END-HTTP---------")
	if err != nil {
		return err
	}

	// Returns success.
	return nil
}

// do - execute http request.
func (c Client) do(req *http.Request) (*http.Response, error) {
	// execute the request.
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return resp, err
	}
	// If trace is enabled, dump http request and response.
	if c.isTraceEnabled {
		err = c.dumpHTTP(req, resp)
		if err != nil {
			return nil, err
		}
	}
	return resp, nil
}

// newRequest - instantiate a new HTTP request for a given method.
func (c Client) newRequest(method string, metadata requestMetadata) (*http.Request, error) {
	// If no method is supplied default to 'POST'.
	if method == "" {
		method = "POST"
	}

	// construct a new target URL.
	targetURL, err := c.makeTargetURL(metadata.bucketName, metadata.objectName, metadata.queryValues)
	if err != nil {
		return nil, err
	}

	// get a new HTTP request for the method.
	req, err := http.NewRequest(method, targetURL.String(), nil)
	if err != nil {
		return nil, err
	}

	// Gather location only if bucketName is present.
	location := "us-east-1" // Default all other requests to "us-east-1".
	if metadata.bucketName != "" {
		location, err = c.getBucketLocation(metadata.bucketName)
		if err != nil {
			return nil, err
		}
	}

	// If presigned request, return quickly.
	if metadata.expires != 0 {
		if c.anonymous {
			return nil, ErrInvalidArgument("Requests cannot be presigned with anonymous credentials.")
		}
		if c.signature.isV2() {
			// Presign URL with signature v2.
			req = PreSignV2(*req, c.accessKeyID, c.secretAccessKey, metadata.expires)
		} else {
			// Presign URL with signature v4.
			req = PreSignV4(*req, c.accessKeyID, c.secretAccessKey, location, metadata.expires)
		}
		return req, nil
	}

	// Set content body if available.
	if metadata.contentBody != nil {
		req.Body = metadata.contentBody
	}

	// set UserAgent for the request.
	c.setUserAgent(req)

	// Set all headers.
	for k, v := range metadata.customHeader {
		req.Header.Set(k, v[0])
	}

	// set incoming content-length.
	if metadata.contentLength > 0 {
		req.ContentLength = metadata.contentLength
	}

	// Set sha256 sum only for non anonymous credentials.
	if !c.anonymous {
		// set sha256 sum for signature calculation only with signature version '4'.
		if c.signature.isV4() {
			req.Header.Set("X-Amz-Content-Sha256", hex.EncodeToString(sum256([]byte{})))
			if metadata.contentSha256Bytes != nil {
				req.Header.Set("X-Amz-Content-Sha256", hex.EncodeToString(metadata.contentSha256Bytes))
			}
		}
	}

	// set md5Sum for content protection.
	if metadata.contentMD5Bytes != nil {
		req.Header.Set("Content-MD5", base64.StdEncoding.EncodeToString(metadata.contentMD5Bytes))
	}

	// Sign the request if not anonymous.
	if !c.anonymous {
		if c.signature.isV2() {
			// Add signature version '2' authorization header.
			req = SignV2(*req, c.accessKeyID, c.secretAccessKey)
		} else if c.signature.isV4() {
			// Add signature version '4' authorization header.
			req = SignV4(*req, c.accessKeyID, c.secretAccessKey, location)
		}
	}
	// return request.
	return req, nil
}

func (c Client) setUserAgent(req *http.Request) {
	req.Header.Set("User-Agent", libraryUserAgent)
	if c.appInfo.appName != "" && c.appInfo.appVersion != "" {
		req.Header.Set("User-Agent", libraryUserAgent+" "+c.appInfo.appName+"/"+c.appInfo.appVersion)
	}
}

func (c Client) makeTargetURL(bucketName, objectName string, queryValues url.Values) (*url.URL, error) {
	urlStr := c.endpointURL.Scheme + "://" + c.endpointURL.Host + "/"
	// Make URL only if bucketName is available, otherwise use the endpoint URL.
	if bucketName != "" {
		// If endpoint supports virtual host style use that always.
		// Currently only S3 and Google Cloud Storage would support this.
		if isVirtualHostSupported(c.endpointURL) {
			urlStr = c.endpointURL.Scheme + "://" + bucketName + "." + c.endpointURL.Host + "/"
			if objectName != "" {
				urlStr = urlStr + urlEncodePath(objectName)
			}
		} else {
			// If not fall back to using path style.
			urlStr = urlStr + bucketName
			if objectName != "" {
				urlStr = urlStr + "/" + urlEncodePath(objectName)
			}
		}
	}
	// If there are any query values, add them to the end.
	if len(queryValues) > 0 {
		urlStr = urlStr + "?" + queryValues.Encode()
	}
	u, err := url.Parse(urlStr)
	if err != nil {
		return nil, err
	}

	return u, nil
}

// CloudStorageClient - Cloud Storage Client interface.
type CloudStorageClient interface {
	// Bucket Read/Write/Stat operations.
	MakeBucket(bucketName string, cannedACL BucketACL, location string) error
	BucketExists(bucketName string) error
	RemoveBucket(bucketName string) error
	SetBucketACL(bucketName string, cannedACL BucketACL) error
	GetBucketACL(bucketName string) (BucketACL, error)

	ListBuckets() ([]BucketStat, error)
	ListObjects(bucket, prefix string, recursive bool, doneCh <-chan struct{}) <-chan ObjectStat
	ListIncompleteUploads(bucket, prefix string, recursive bool, doneCh <-chan struct{}) <-chan ObjectMultipartStat

	// Object Read/Write/Stat operations.
	GetObject(bucketName, objectName string) (reader io.ReadCloser, stat ObjectStat, err error)
	PutObject(bucketName, objectName string, data io.Reader, size int64, contentType string) (n int64, err error)
	StatObject(bucketName, objectName string) (ObjectStat, error)
	RemoveObject(bucketName, objectName string) error
	RemoveIncompleteUpload(bucketName, objectName string) error

	// Object Read/Write for sparse upload.
	GetObjectPartial(bucketName, objectName string) (reader ReadAtCloser, stat ObjectStat, err error)
	PutObjectPartial(bucketName, objectName string, data ReadAtCloser, size int64, contentType string) (n int64, err error)

	// File to Object API.
	FPutObject(bucketName, objectName, filePath, contentType string) (n int64, err error)
	FGetObject(bucketName, objectName, filePath string) error

	// Presigned operations.
	PresignedGetObject(bucketName, objectName string, expires time.Duration) (presignedURL string, err error)
	PresignedPutObject(bucketName, objectName string, expires time.Duration) (presignedURL string, err error)
	PresignedPostPolicy(*PostPolicy) (formData map[string]string, err error)

	// Application info.
	SetAppInfo(appName, appVersion string)

	// Set custom transport.
	SetCustomTransport(customTransport http.RoundTripper)

	// HTTP tracing methods.
	TraceOn(traceOutput io.Writer) error
	TraceOff()
}
