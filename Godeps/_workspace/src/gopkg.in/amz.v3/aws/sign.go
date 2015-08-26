package aws

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

var debug = log.New(
	// Remove the c-style comment header to front of line to debug information.
	/*os.Stdout, //*/ ioutil.Discard,
	"DEBUG: ",
	log.LstdFlags,
)

type Signer func(*http.Request, Auth) error

// Ensure our signers meet the interface
var _ Signer = SignV2
var _ Signer = SignV4Factory("", "")

type hasher func(io.Reader) (string, error)

const (
	ISO8601BasicFormat      = "20060102T150405Z"
	ISO8601BasicFormatShort = "20060102"
)

// SignV2 signs an HTTP request utilizing version 2 of the AWS
// signature, and the given credentials.
func SignV2(req *http.Request, auth Auth) (err error) {

	queryVals := req.URL.Query()
	queryVals.Set("AWSAccessKeyId", auth.AccessKey)
	queryVals.Set("SignatureVersion", "2")
	queryVals.Set("SignatureMethod", "HmacSHA256")

	uriStr := canonicalURI(req.URL)
	queryStr := canonicalQueryString(queryVals)

	payload := new(bytes.Buffer)
	if err := errorCollector(
		fprintfWrapper(payload, "%s\n", requestMethodVerb(req.Method)),
		fprintfWrapper(payload, "%s\n", req.Host),
		fprintfWrapper(payload, "%s\n", uriStr),
		fprintfWrapper(payload, "%s", queryStr),
	); err != nil {
		return err
	}

	hash := hmac.New(sha256.New, []byte(auth.SecretKey))
	hash.Write(payload.Bytes())
	signature := make([]byte, base64.StdEncoding.EncodedLen(hash.Size()))
	base64.StdEncoding.Encode(signature, hash.Sum(nil))

	queryVals.Set("Signature", string(signature))
	req.URL.RawQuery = queryVals.Encode()

	return nil
}

// SignV4Factory returns a version 4 Signer which will utilize the
// given region name.
func SignV4Factory(regionName, serviceName string) Signer {
	return func(req *http.Request, auth Auth) error {
		return SignV4(req, auth, regionName, serviceName)
	}
}

func SignV4URL(req *http.Request, auth Auth, regionName, svcName string, expires time.Duration) error {
	reqTime, err := requestTime(req)
	if err != nil {
		return err
	}

	req.Header.Del("date")

	credScope := credentialScope(reqTime, regionName, svcName)

	queryVals := req.URL.Query()
	queryVals.Set("X-Amz-Algorithm", "AWS4-HMAC-SHA256")
	queryVals.Set("X-Amz-Credential", auth.AccessKey+"/"+credScope)
	queryVals.Set("X-Amz-Date", reqTime.Format(ISO8601BasicFormat))
	queryVals.Set("X-Amz-Expires", fmt.Sprintf("%d", int(expires.Seconds())))
	queryVals.Set("X-Amz-SignedHeaders", "host")
	req.URL.RawQuery = queryVals.Encode()

	_, canonReqHash, _, err := canonicalRequest(req, sha256Hasher, false)
	if err != nil {
		return err
	}

	var strToSign string
	if strToSign, err = stringToSign(reqTime, canonReqHash, credScope); err != nil {
		return err
	}

	key := signingKey(reqTime, auth.SecretKey, regionName, svcName)
	signature := fmt.Sprintf("%x", hmacHasher(key, strToSign))

	debug.Printf("strToSign:\n\"\"\"\n%s\n\"\"\"", strToSign)

	queryVals.Set("X-Amz-Signature", signature)

	req.URL.RawQuery = queryVals.Encode()

	return nil
}

// SignV4 signs an HTTP request utilizing version 4 of the AWS
// signature, and the given credentials.
func SignV4(req *http.Request, auth Auth, regionName, svcName string) (err error) {

	var reqTime time.Time
	if reqTime, err = requestTime(req); err != nil {
		return err
	}

	// Remove any existing authorization headers as they will corrupt
	// the signing.
	delete(req.Header, "Authorization")
	delete(req.Header, "authorization")

	credScope := credentialScope(reqTime, regionName, svcName)

	_, canonReqHash, sortedHdrNames, err := canonicalRequest(req, sha256Hasher, true)
	if err != nil {
		return err
	}

	var strToSign string
	if strToSign, err = stringToSign(reqTime, canonReqHash, credScope); err != nil {
		return err
	}

	key := signingKey(reqTime, auth.SecretKey, regionName, svcName)
	signature := fmt.Sprintf("%x", hmacHasher(key, strToSign))

	debug.Printf("strToSign:\n\"\"\"\n%s\n\"\"\"", strToSign)

	var authHdrVal string
	if authHdrVal, err = authHeaderString(
		req.Header,
		auth.AccessKey,
		signature,
		credScope,
		sortedHdrNames,
	); err != nil {
		return err
	}

	req.Header.Set("Authorization", authHdrVal)

	return nil
}

// Task 1: Create a Canonical Request.
// Returns the canonical request, and its hash.
func canonicalRequest(
	req *http.Request,
	hasher hasher,
	calcPayHash bool,
) (canReq, canReqHash string, sortedHdrNames []string, err error) {

	payHash := "UNSIGNED-PAYLOAD"
	if calcPayHash {
		if payHash, err = payloadHash(req, hasher); err != nil {
			return
		}
		req.Header.Set("x-amz-content-sha256", payHash)
	}

	sortedHdrNames = sortHeaderNames(req.Header, "host")
	var canHdr string
	if canHdr, err = canonicalHeaders(sortedHdrNames, req.Host, req.Header); err != nil {
		return
	}

	debug.Printf("canHdr:\n\"\"\"\n%s\n\"\"\"", canHdr)
	debug.Printf("signedHeader: %s\n\n", strings.Join(sortedHdrNames, ";"))

	uriStr := canonicalURI(req.URL)
	queryStr := canonicalQueryString(req.URL.Query())

	c := new(bytes.Buffer)
	if err := errorCollector(
		fprintfWrapper(c, "%s\n", requestMethodVerb(req.Method)),
		fprintfWrapper(c, "%s\n", uriStr),
		fprintfWrapper(c, "%s\n", queryStr),
		fprintfWrapper(c, "%s\n", canHdr),
		fprintfWrapper(c, "%s\n", strings.Join(sortedHdrNames, ";")),
		fprintfWrapper(c, "%s", payHash),
	); err != nil {
		return "", "", nil, err
	}

	canReq = c.String()
	debug.Printf("canReq:\n\"\"\"\n%s\n\"\"\"", canReq)
	canReqHash, err = hasher(bytes.NewBuffer([]byte(canReq)))

	return canReq, canReqHash, sortedHdrNames, err
}

// Task 2: Create a string to Sign
// Returns a string in the defined format to sign for the authorization header.
func stringToSign(
	t time.Time,
	canonReqHash string,
	credScope string,
) (string, error) {
	w := new(bytes.Buffer)
	if err := errorCollector(
		fprintfWrapper(w, "AWS4-HMAC-SHA256\n"),
		fprintfWrapper(w, "%s\n", t.Format(ISO8601BasicFormat)),
		fprintfWrapper(w, "%s\n", credScope),
		fprintfWrapper(w, "%s", canonReqHash),
	); err != nil {
		return "", err
	}

	return w.String(), nil
}

// Task 3: Calculate the Signature
// Returns a derived signing key.
func signingKey(t time.Time, secretKey, regionName, svcName string) []byte {

	kSecret := secretKey
	kDate := hmacHasher([]byte("AWS4"+kSecret), t.Format(ISO8601BasicFormatShort))
	kRegion := hmacHasher(kDate, regionName)
	kService := hmacHasher(kRegion, svcName)
	kSigning := hmacHasher(kService, "aws4_request")

	return kSigning
}

// Task 4: Add the Signing Information to the Request
// Returns a string to be placed in the Authorization header for the request.
func authHeaderString(
	header http.Header,
	accessKey,
	signature string,
	credScope string,
	sortedHeaderNames []string,
) (string, error) {
	w := new(bytes.Buffer)
	if err := errorCollector(
		fprintfWrapper(w, "AWS4-HMAC-SHA256 "),
		fprintfWrapper(w, "Credential=%s/%s, ", accessKey, credScope),
		fprintfWrapper(w, "SignedHeaders=%s, ", strings.Join(sortedHeaderNames, ";")),
		fprintfWrapper(w, "Signature=%s", signature),
	); err != nil {
		return "", err
	}

	return w.String(), nil
}

func canonicalURI(u *url.URL) string {

	// The algorithm states that if the path is empty, to just use a "/".
	if u.Path == "" {
		return "/"
	}

	// Each path segment must be URI-encoded.
	segments := strings.Split(u.Path, "/")
	for i, segment := range segments {
		segments[i] = goToAwsUrlEncoding(url.QueryEscape(segment))
	}

	return strings.Join(segments, "/")
}

func canonicalQueryString(queryVals url.Values) string {

	// AWS dictates that if duplicate keys exist, their values be
	// sorted as well.
	for _, values := range queryVals {
		sort.Strings(values)
	}

	return goToAwsUrlEncoding(queryVals.Encode())
}

func goToAwsUrlEncoding(urlEncoded string) string {
	// AWS dictates that we use %20 for encoding spaces rather than +.
	// All significant +s should already be encoded into their
	// hexadecimal equivalents before doing the string replace.
	return strings.Replace(urlEncoded, "+", "%20", -1)
}

func canonicalHeaders(sortedHeaderNames []string, host string, hdr http.Header) (string, error) {
	buffer := new(bytes.Buffer)

	for _, hName := range sortedHeaderNames {

		hdrVals := host
		if hName != "host" {
			canonHdrKey := http.CanonicalHeaderKey(hName)
			sortedHdrVals := hdr[canonHdrKey]
			sort.Strings(sortedHdrVals)
			hdrVals = strings.Join(sortedHdrVals, ",")
		}

		if _, err := fmt.Fprintf(buffer, "%s:%s\n", hName, hdrVals); err != nil {
			return "", err
		}
	}

	// There is intentionally a hanging newline at the end of the
	// header list.
	return buffer.String(), nil
}

// Returns a SHA256 checksum of the request body. Represented as a
// lowercase hexadecimal string.
func payloadHash(req *http.Request, hasher hasher) (string, error) {
	if req.Body == nil {
		return hasher(bytes.NewBuffer(nil))
	}

	return hasher(req.Body)
}

// Retrieve the header names, lower-case them, and sort them.
func sortHeaderNames(header http.Header, injectedNames ...string) []string {

	sortedNames := injectedNames
	for hName, _ := range header {
		sortedNames = append(sortedNames, strings.ToLower(hName))
	}

	sort.Strings(sortedNames)

	return sortedNames
}

func hmacHasher(key []byte, value string) []byte {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(value))
	return h.Sum(nil)
}

func sha256Hasher(payloadReader io.Reader) (string, error) {
	hasher := sha256.New()
	_, err := io.Copy(hasher, payloadReader)

	return fmt.Sprintf("%x", hasher.Sum(nil)), err
}

func credentialScope(t time.Time, regionName, svcName string) string {
	return fmt.Sprintf(
		"%s/%s/%s/aws4_request",
		t.Format(ISO8601BasicFormatShort),
		regionName,
		svcName,
	)
}

// We do a lot of fmt.Fprintfs in this package. Create a higher-order
// function to elide the bytes written return value so we can submit
// these calls to an error collector.
func fprintfWrapper(w io.Writer, format string, vals ...interface{}) func() error {
	return func() error {
		_, err := fmt.Fprintf(w, format, vals...)
		return err
	}
}

// Poor man's maybe monad.
func errorCollector(writers ...func() error) error {
	for _, writer := range writers {
		if err := writer(); err != nil {
			return err
		}
	}

	return nil
}

// Retrieve the request time from the request. We will attempt to
// parse whatever we find, but we will not make up a request date for
// the user (i.e.: Magic!).
func requestTime(req *http.Request) (time.Time, error) {

	// Time formats to try. We want to do everything we can to accept
	// all time formats, but ultimately we may fail. In the package
	// scope so it doesn't get initialized for every request.
	var timeFormats = []string{
		time.RFC822,
		ISO8601BasicFormat,
		time.RFC1123,
		time.ANSIC,
		time.UnixDate,
		time.RubyDate,
		time.RFC822Z,
		time.RFC850,
		time.RFC1123Z,
		time.RFC3339,
		time.RFC3339Nano,
		time.Kitchen,
	}

	// Get a date header.
	var date string
	if date = req.Header.Get("x-amz-date"); date == "" {
		if date = req.Header.Get("date"); date == "" {
			return time.Time{}, fmt.Errorf(`Could not retrieve a request date. Please provide one in either "x-amz-date", or "date".`)
		}
	}

	// Start attempting to parse
	for _, format := range timeFormats {
		if parsedTime, err := time.Parse(format, date); err == nil {
			return parsedTime, nil
		}
	}

	return time.Time{}, fmt.Errorf(
		"Could not parse the given date. Please utilize one of the following formats: %s",
		strings.Join(timeFormats, ","),
	)
}

// http.Request's Method member returns the entire method. Derive the
// verb.
func requestMethodVerb(rawMethod string) (verb string) {
	verbPlus := strings.SplitN(rawMethod, " ", 2)
	switch {
	case len(verbPlus) == 0: // Per docs, Method will be empty if it's GET.
		verb = "GET"
	default:
		verb = verbPlus[0]
	}
	return verb
}
