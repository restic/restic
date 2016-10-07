// This tests the swift packagae
//
// It can be used with a real swift server which should be set up in
// the environment variables SWIFT_API_USER, SWIFT_API_KEY and
// SWIFT_AUTH_URL
// In case those variables are not defined, a fake Swift server
// is used instead - see Testing in README.md for more info
//
// The functions are designed to run in order and create things the
// next function tests.  This means that if it goes wrong it is likely
// errors will propagate.  You may need to tidy up the CONTAINER to
// get it to run cleanly.
package swift_test

import (
	"archive/tar"
	"bytes"
	"crypto/md5"
	"crypto/tls"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ncw/swift"
	"github.com/ncw/swift/swifttest"
)

var (
	c                *swift.Connection
	srv              *swifttest.SwiftServer
	m1               = swift.Metadata{"Hello": "1", "potato-Salad": "2"}
	m2               = swift.Metadata{"hello": "", "potato-salad": ""}
	skipVersionTests = false
)

const (
	CONTAINER          = "GoSwiftUnitTest"
	VERSIONS_CONTAINER = "GoSwiftUnitTestVersions"
	CURRENT_CONTAINER  = "GoSwiftUnitTestCurrent"
	OBJECT             = "test_object"
	OBJECT2            = "test_object2"
	EMPTYOBJECT        = "empty_test_object"
	CONTENTS           = "12345"
	CONTENTS2          = "54321"
	CONTENT_SIZE       = int64(len(CONTENTS))
	CONTENT_MD5        = "827ccb0eea8a706c4c34a16891f84e7b"
	EMPTY_MD5          = "d41d8cd98f00b204e9800998ecf8427e"
	SECRET_KEY         = "b3968d0207b54ece87cccc06515a89d4"
)

type someTransport struct{ http.Transport }

func makeConnection() (*swift.Connection, error) {
	var err error

	UserName := os.Getenv("SWIFT_API_USER")
	ApiKey := os.Getenv("SWIFT_API_KEY")
	AuthUrl := os.Getenv("SWIFT_AUTH_URL")
	Region := os.Getenv("SWIFT_REGION_NAME")
	EndpointType := os.Getenv("SWIFT_ENDPOINT_TYPE")

	Insecure := os.Getenv("SWIFT_AUTH_INSECURE")
	ConnectionChannelTimeout := os.Getenv("SWIFT_CONNECTION_CHANNEL_TIMEOUT")
	DataChannelTimeout := os.Getenv("SWIFT_DATA_CHANNEL_TIMEOUT")

	if UserName == "" || ApiKey == "" || AuthUrl == "" {
		if srv != nil {
			srv.Close()
		}
		srv, err = swifttest.NewSwiftServer("localhost")
		if err != nil {
			return nil, err
		}

		UserName = "swifttest"
		ApiKey = "swifttest"
		AuthUrl = srv.AuthURL
	}

	transport := &http.Transport{
		Proxy:               http.ProxyFromEnvironment,
		MaxIdleConnsPerHost: 2048,
	}
	if Insecure == "1" {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}

	c := swift.Connection{
		UserName:       UserName,
		ApiKey:         ApiKey,
		AuthUrl:        AuthUrl,
		Region:         Region,
		Transport:      transport,
		ConnectTimeout: 60 * time.Second,
		Timeout:        60 * time.Second,
		EndpointType:   swift.EndpointType(EndpointType),
	}

	var timeout int64
	if ConnectionChannelTimeout != "" {
		timeout, err = strconv.ParseInt(ConnectionChannelTimeout, 10, 32)
		if err == nil {
			c.ConnectTimeout = time.Duration(timeout) * time.Second
		}
	}

	if DataChannelTimeout != "" {
		timeout, err = strconv.ParseInt(DataChannelTimeout, 10, 32)
		if err == nil {
			c.Timeout = time.Duration(timeout) * time.Second
		}
	}

	return &c, nil
}

func isV3Api() bool {
	AuthUrl := os.Getenv("SWIFT_AUTH_URL")
	return strings.Contains(AuthUrl, "v3")
}

func TestTransport(t *testing.T) {
	var err error

	c, err = makeConnection()
	if err != nil {
		t.Fatal("Failed to create server", err)
	}

	tr := &someTransport{
		Transport: http.Transport{
			MaxIdleConnsPerHost: 2048,
		},
	}

	Insecure := os.Getenv("SWIFT_AUTH_INSECURE")

	if Insecure == "1" {
		tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}

	c.Transport = tr

	if isV3Api() {
		c.Tenant = os.Getenv("SWIFT_TENANT")
		c.Domain = os.Getenv("SWIFT_API_DOMAIN")
	} else {
		c.Tenant = os.Getenv("SWIFT_TENANT")
		c.TenantId = os.Getenv("SWIFT_TENANT_ID")
	}

	err = c.Authenticate()
	if err != nil {
		t.Fatal("Auth failed", err)
	}
	if !c.Authenticated() {
		t.Fatal("Not authenticated")
	}
	if srv != nil {
		srv.Close()
	}
}

// The following Test functions are run in order - this one must come before the others!
func TestV1V2Authenticate(t *testing.T) {
	var err error

	if isV3Api() {
		return
	}

	c, err = makeConnection()
	if err != nil {
		t.Fatal("Failed to create server", err)
	}

	c.Tenant = os.Getenv("SWIFT_TENANT")
	c.TenantId = os.Getenv("SWIFT_TENANT_ID")

	err = c.Authenticate()
	if err != nil {
		t.Fatal("Auth failed", err)
	}
	if !c.Authenticated() {
		t.Fatal("Not authenticated")
	}
}

func TestV3AuthenticateWithDomainNameAndTenantId(t *testing.T) {
	var err error
	if !isV3Api() {
		return
	}

	c, err = makeConnection()
	if err != nil {
		t.Fatal("Failed to create server", err)
	}

	c.TenantId = os.Getenv("SWIFT_TENANT_ID")
	c.Domain = os.Getenv("SWIFT_API_DOMAIN")

	err = c.Authenticate()
	if err != nil {
		t.Fatal("Auth failed", err)
	}
	if !c.Authenticated() {
		t.Fatal("Not authenticated")
	}
}

func TestV3TrustWithTrustId(t *testing.T) {
	var err error
	if !isV3Api() {
		return
	}

	c, err = makeConnection()
	if err != nil {
		t.Fatal("Failed to create server", err)
	}

	c.TrustId = os.Getenv("SWIFT_TRUST_ID")

	err = c.Authenticate()
	if err != nil {
		t.Fatal("Auth failed", err)
	}
	if !c.Authenticated() {
		t.Fatal("Not authenticated")
	}
}

func TestV3AuthenticateWithDomainIdAndTenantId(t *testing.T) {
	var err error

	if !isV3Api() {
		return
	}

	c, err = makeConnection()
	if err != nil {
		t.Fatal("Failed to create server", err)
	}

	c.TenantId = os.Getenv("SWIFT_TENANT_ID")
	c.DomainId = os.Getenv("SWIFT_API_DOMAIN_ID")

	err = c.Authenticate()
	if err != nil {
		t.Fatal("Auth failed", err)
	}
	if !c.Authenticated() {
		t.Fatal("Not authenticated")
	}
}

func TestV3AuthenticateWithDomainNameAndTenantName(t *testing.T) {
	var err error

	if !isV3Api() {
		return
	}

	c, err = makeConnection()
	if err != nil {
		t.Fatal("Failed to create server", err)
	}

	c.Tenant = os.Getenv("SWIFT_TENANT")
	c.Domain = os.Getenv("SWIFT_API_DOMAIN")

	err = c.Authenticate()
	if err != nil {
		t.Fatal("Auth failed", err)
	}
	if !c.Authenticated() {
		t.Fatal("Not authenticated")
	}
}

func TestV3AuthenticateWithDomainIdAndTenantName(t *testing.T) {
	var err error

	if !isV3Api() {
		return
	}

	c, err = makeConnection()
	if err != nil {
		t.Fatal("Failed to create server", err)
	}

	c.Tenant = os.Getenv("SWIFT_TENANT")
	c.DomainId = os.Getenv("SWIFT_API_DOMAIN_ID")

	err = c.Authenticate()
	if err != nil {
		t.Fatal("Auth failed", err)
	}
	if !c.Authenticated() {
		t.Fatal("Not authenticated")
	}
}

// Attempt to trigger a race in authenticate
//
// Run with -race to test
func TestAuthenticateRace(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := c.Authenticate()
			if err != nil {
				t.Fatal("Auth failed", err)
			}
			if !c.Authenticated() {
				t.Fatal("Not authenticated")
			}
		}()
	}
	wg.Wait()
}

// Test a connection can be serialized and unserialized with JSON
func TestSerializeConnectionJson(t *testing.T) {
	serializedConnection, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("Failed to serialize connection: %v", err)
	}
	c2 := new(swift.Connection)
	err = json.Unmarshal(serializedConnection, &c2)
	if err != nil {
		t.Fatalf("Failed to unserialize connection: %v", err)
	}
	if !c2.Authenticated() {
		t.Fatal("Should be authenticated")
	}
	_, _, err = c2.Account()
	if err != nil {
		t.Fatalf("Failed to use unserialized connection: %v", err)
	}
}

// Test a connection can be serialized and unserialized with XML
func TestSerializeConnectionXml(t *testing.T) {
	serializedConnection, err := xml.Marshal(c)
	if err != nil {
		t.Fatalf("Failed to serialize connection: %v", err)
	}
	c2 := new(swift.Connection)
	err = xml.Unmarshal(serializedConnection, &c2)
	if err != nil {
		t.Fatalf("Failed to unserialize connection: %v", err)
	}
	if !c2.Authenticated() {
		t.Fatal("Should be authenticated")
	}
	_, _, err = c2.Account()
	if err != nil {
		t.Fatalf("Failed to use unserialized connection: %v", err)
	}
}

// Test the reauthentication logic
func TestOnReAuth(t *testing.T) {
	c2 := c
	c2.UnAuthenticate()
	_, _, err := c2.Account()
	if err != nil {
		t.Fatalf("Failed to reauthenticate: %v", err)
	}
}
func TestAccount(t *testing.T) {
	info, headers, err := c.Account()
	if err != nil {
		t.Fatal(err)
	}
	if headers["X-Account-Container-Count"] != fmt.Sprintf("%d", info.Containers) {
		t.Error("Bad container count")
	}
	if headers["X-Account-Bytes-Used"] != fmt.Sprintf("%d", info.BytesUsed) {
		t.Error("Bad bytes count")
	}
	if headers["X-Account-Object-Count"] != fmt.Sprintf("%d", info.Objects) {
		t.Error("Bad objects count")
	}
	//fmt.Println(info)
	//fmt.Println(headers)
}

func compareMaps(t *testing.T, a, b map[string]string) {
	if len(a) != len(b) {
		t.Error("Maps different sizes", a, b)
	}
	for ka, va := range a {
		if vb, ok := b[ka]; !ok || va != vb {
			t.Error("Difference in key", ka, va, b[ka])
		}
	}
	for kb, vb := range b {
		if va, ok := a[kb]; !ok || vb != va {
			t.Error("Difference in key", kb, vb, a[kb])
		}
	}
}

func TestAccountUpdate(t *testing.T) {
	err := c.AccountUpdate(m1.AccountHeaders())
	if err != nil {
		t.Fatal(err)
	}

	_, headers, err := c.Account()
	if err != nil {
		t.Fatal(err)
	}
	m := headers.AccountMetadata()
	delete(m, "temp-url-key") // remove X-Account-Meta-Temp-URL-Key if set
	compareMaps(t, m, map[string]string{"hello": "1", "potato-salad": "2"})

	err = c.AccountUpdate(m2.AccountHeaders())
	if err != nil {
		t.Fatal(err)
	}

	_, headers, err = c.Account()
	if err != nil {
		t.Fatal(err)
	}
	m = headers.AccountMetadata()
	delete(m, "temp-url-key") // remove X-Account-Meta-Temp-URL-Key if set
	compareMaps(t, m, map[string]string{})

	//fmt.Println(c.Account())
	//fmt.Println(headers)
	//fmt.Println(headers.AccountMetadata())
	//fmt.Println(c.AccountUpdate(m2.AccountHeaders()))
	//fmt.Println(c.Account())
}

func TestContainerCreate(t *testing.T) {
	err := c.ContainerCreate(CONTAINER, m1.ContainerHeaders())
	if err != nil {
		t.Fatal(err)
	}
}

func TestContainer(t *testing.T) {
	info, headers, err := c.Container(CONTAINER)
	if err != nil {
		t.Fatal(err)
	}
	compareMaps(t, headers.ContainerMetadata(), map[string]string{"hello": "1", "potato-salad": "2"})
	if CONTAINER != info.Name {
		t.Error("Bad container count")
	}
	if headers["X-Container-Bytes-Used"] != fmt.Sprintf("%d", info.Bytes) {
		t.Error("Bad bytes count")
	}
	if headers["X-Container-Object-Count"] != fmt.Sprintf("%d", info.Count) {
		t.Error("Bad objects count")
	}
	//fmt.Println(info)
	//fmt.Println(headers)
}

func TestContainersAll(t *testing.T) {
	containers1, err := c.ContainersAll(nil)
	if err != nil {
		t.Fatal(err)
	}
	containers2, err := c.Containers(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(containers1) != len(containers2) {
		t.Fatal("Wrong length")
	}
	for i := range containers1 {
		if containers1[i] != containers2[i] {
			t.Fatal("Not the same")
		}
	}
}

func TestContainersAllWithLimit(t *testing.T) {
	containers1, err := c.ContainersAll(&swift.ContainersOpts{Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	containers2, err := c.Containers(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(containers1) != len(containers2) {
		t.Fatal("Wrong length")
	}
	for i := range containers1 {
		if containers1[i] != containers2[i] {
			t.Fatal("Not the same")
		}
	}
}

func TestContainerUpdate(t *testing.T) {
	err := c.ContainerUpdate(CONTAINER, m2.ContainerHeaders())
	if err != nil {
		t.Fatal(err)
	}
	_, headers, err := c.Container(CONTAINER)
	if err != nil {
		t.Fatal(err)
	}
	compareMaps(t, headers.ContainerMetadata(), map[string]string{})
	//fmt.Println(headers)
}

func TestContainerNames(t *testing.T) {
	containers, err := c.ContainerNames(nil)
	if err != nil {
		t.Fatal(err)
	}
	// fmt.Printf("container %q\n", CONTAINER)
	ok := false
	for _, container := range containers {
		if container == CONTAINER {
			ok = true
			break
		}
	}
	if !ok {
		t.Errorf("Didn't find container %q in listing %q", CONTAINER, containers)
	}
	// fmt.Println(containers)
}

func TestContainerNamesAll(t *testing.T) {
	containers1, err := c.ContainerNamesAll(nil)
	if err != nil {
		t.Fatal(err)
	}
	containers2, err := c.ContainerNames(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(containers1) != len(containers2) {
		t.Fatal("Wrong length")
	}
	for i := range containers1 {
		if containers1[i] != containers2[i] {
			t.Fatal("Not the same")
		}
	}
}

func TestContainerNamesAllWithLimit(t *testing.T) {
	containers1, err := c.ContainerNamesAll(&swift.ContainersOpts{Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	containers2, err := c.ContainerNames(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(containers1) != len(containers2) {
		t.Fatal("Wrong length")
	}
	for i := range containers1 {
		if containers1[i] != containers2[i] {
			t.Fatal("Not the same")
		}
	}
}

func TestObjectPutString(t *testing.T) {
	err := c.ObjectPutString(CONTAINER, OBJECT, CONTENTS, "")
	if err != nil {
		t.Fatal(err)
	}

	info, _, err := c.Object(CONTAINER, OBJECT)
	if err != nil {
		t.Error(err)
	}
	if info.ContentType != "application/octet-stream" {
		t.Error("Bad content type", info.ContentType)
	}
	if info.Bytes != CONTENT_SIZE {
		t.Error("Bad length")
	}
	if info.Hash != CONTENT_MD5 {
		t.Error("Bad length")
	}
}

func TestObjectPut(t *testing.T) {
	headers := swift.Headers{}

	// Set content size incorrectly - should produce an error
	headers["Content-Length"] = strconv.FormatInt(CONTENT_SIZE-1, 10)
	contents := bytes.NewBufferString(CONTENTS)
	h, err := c.ObjectPut(CONTAINER, OBJECT, contents, true, CONTENT_MD5, "text/plain", headers)
	if err == nil {
		t.Fatal("Expecting error but didn't get one")
	}

	// Now set content size correctly
	contents = bytes.NewBufferString(CONTENTS)
	headers["Content-Length"] = strconv.FormatInt(CONTENT_SIZE, 10)
	h, err = c.ObjectPut(CONTAINER, OBJECT, contents, true, CONTENT_MD5, "text/plain", headers)
	if err != nil {
		t.Fatal(err)
	}

	if h["Etag"] != CONTENT_MD5 {
		t.Errorf("Bad Etag want %q got %q", CONTENT_MD5, h["Etag"])
	}

	// Fetch object info and compare
	info, _, err := c.Object(CONTAINER, OBJECT)
	if err != nil {
		t.Error(err)
	}
	if info.ContentType != "text/plain" {
		t.Error("Bad content type", info.ContentType)
	}
	if info.Bytes != CONTENT_SIZE {
		t.Error("Bad length")
	}
	if info.Hash != CONTENT_MD5 {
		t.Error("Bad length")
	}
}

func TestObjectEmpty(t *testing.T) {
	err := c.ObjectPutString(CONTAINER, EMPTYOBJECT, "", "")
	if err != nil {
		t.Fatal(err)
	}

	info, _, err := c.Object(CONTAINER, EMPTYOBJECT)
	if err != nil {
		t.Error(err)
	}
	if info.ContentType != "application/octet-stream" {
		t.Error("Bad content type", info.ContentType)
	}
	if info.Bytes != 0 {
		t.Errorf("Bad length want 0 got %v", info.Bytes)
	}
	if info.Hash != EMPTY_MD5 {
		t.Errorf("Bad MD5 want %v got %v", EMPTY_MD5, info.Hash)
	}

	// Tidy up
	err = c.ObjectDelete(CONTAINER, EMPTYOBJECT)
	if err != nil {
		t.Error(err)
	}
}

func TestObjectPutBytes(t *testing.T) {
	err := c.ObjectPutBytes(CONTAINER, OBJECT, []byte(CONTENTS), "")
	if err != nil {
		t.Fatal(err)
	}

	info, _, err := c.Object(CONTAINER, OBJECT)
	if err != nil {
		t.Error(err)
	}
	if info.ContentType != "application/octet-stream" {
		t.Error("Bad content type", info.ContentType)
	}
	if info.Bytes != CONTENT_SIZE {
		t.Error("Bad length")
	}
	if info.Hash != CONTENT_MD5 {
		t.Error("Bad length")
	}
}

func TestObjectPutMimeType(t *testing.T) {
	err := c.ObjectPutString(CONTAINER, "test.jpg", CONTENTS, "")
	if err != nil {
		t.Fatal(err)
	}

	info, _, err := c.Object(CONTAINER, "test.jpg")
	if err != nil {
		t.Error(err)
	}
	if info.ContentType != "image/jpeg" {
		t.Error("Bad content type", info.ContentType)
	}

	// Tidy up
	err = c.ObjectDelete(CONTAINER, "test.jpg")
	if err != nil {
		t.Error(err)
	}
}

func TestObjectCreate(t *testing.T) {
	out, err := c.ObjectCreate(CONTAINER, OBJECT2, true, "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	buf := &bytes.Buffer{}
	hash := md5.New()
	out2 := io.MultiWriter(out, buf, hash)
	for i := 0; i < 100; i++ {
		fmt.Fprintf(out2, "%d %s\n", i, CONTENTS)
	}
	err = out.Close()
	if err != nil {
		t.Error(err)
	}
	expected := buf.String()
	contents, err := c.ObjectGetString(CONTAINER, OBJECT2)
	if err != nil {
		t.Error(err)
	}
	if contents != expected {
		t.Error("Contents wrong")
	}

	// Test writing on closed file
	n, err := out.Write([]byte{0})
	if err == nil || n != 0 {
		t.Error("Expecting error and n == 0 writing on closed file", err, n)
	}

	// Now with hash instead
	out, err = c.ObjectCreate(CONTAINER, OBJECT2, false, fmt.Sprintf("%x", hash.Sum(nil)), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = out.Write(buf.Bytes())
	if err != nil {
		t.Error(err)
	}
	err = out.Close()
	if err != nil {
		t.Error(err)
	}
	contents, err = c.ObjectGetString(CONTAINER, OBJECT2)
	if err != nil {
		t.Error(err)
	}
	if contents != expected {
		t.Error("Contents wrong")
	}

	// Now with bad hash
	out, err = c.ObjectCreate(CONTAINER, OBJECT2, false, CONTENT_MD5, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	// FIXME: work around bug which produces 503 not 422 for empty corrupted files
	fmt.Fprintf(out, "Sausage")
	err = out.Close()
	if err != swift.ObjectCorrupted {
		t.Error("Expecting object corrupted not", err)
	}

	// Tidy up
	err = c.ObjectDelete(CONTAINER, OBJECT2)
	if err != nil {
		t.Error(err)
	}
}

func TestObjectGetString(t *testing.T) {
	contents, err := c.ObjectGetString(CONTAINER, OBJECT)
	if err != nil {
		t.Fatal(err)
	}
	if contents != CONTENTS {
		t.Error("Contents wrong")
	}
	//fmt.Println(contents)
}

func TestObjectGetBytes(t *testing.T) {
	contents, err := c.ObjectGetBytes(CONTAINER, OBJECT)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != CONTENTS {
		t.Error("Contents wrong")
	}
	//fmt.Println(contents)
}

func TestObjectOpen(t *testing.T) {
	file, _, err := c.ObjectOpen(CONTAINER, OBJECT, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	n, err := io.Copy(&buf, file)
	if err != nil {
		t.Fatal(err)
	}
	if n != CONTENT_SIZE {
		t.Fatal("Wrong length", n, CONTENT_SIZE)
	}
	if buf.String() != CONTENTS {
		t.Error("Contents wrong")
	}
	err = file.Close()
	if err != nil {
		t.Fatal(err)
	}
}

func TestObjectOpenPartial(t *testing.T) {
	file, _, err := c.ObjectOpen(CONTAINER, OBJECT, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	n, err := io.CopyN(&buf, file, 1)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatal("Wrong length", n, CONTENT_SIZE)
	}
	if buf.String() != CONTENTS[:1] {
		t.Error("Contents wrong")
	}
	err = file.Close()
	if err != nil {
		t.Fatal(err)
	}
}

func TestObjectOpenLength(t *testing.T) {
	file, _, err := c.ObjectOpen(CONTAINER, OBJECT, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	// FIXME ideally this would check both branches of the Length() code
	n, err := file.Length()
	if err != nil {
		t.Fatal(err)
	}
	if n != CONTENT_SIZE {
		t.Fatal("Wrong length", n, CONTENT_SIZE)
	}
	err = file.Close()
	if err != nil {
		t.Fatal(err)
	}
}

func TestObjectOpenSeek(t *testing.T) {

	plan := []struct {
		whence int
		offset int64
		result int64
	}{
		{-1, 0, 0},
		{-1, 0, 1},
		{-1, 0, 2},
		{0, 0, 0},
		{0, 0, 0},
		{0, 1, 1},
		{0, 2, 2},
		{1, 0, 3},
		{1, -2, 2},
		{1, 1, 4},
		{2, -1, 4},
		{2, -3, 2},
		{2, -2, 3},
		{2, -5, 0},
		{2, -4, 1},
	}

	file, _, err := c.ObjectOpen(CONTAINER, OBJECT, true, nil)
	if err != nil {
		t.Fatal(err)
	}

	for _, p := range plan {
		if p.whence >= 0 {
			result, err := file.Seek(p.offset, p.whence)
			if err != nil {
				t.Fatal(err, p)
			}
			if result != p.result {
				t.Fatal("Seek result was", result, "expecting", p.result, p)
			}

		}
		var buf bytes.Buffer
		n, err := io.CopyN(&buf, file, 1)
		if err != nil {
			t.Fatal(err, p)
		}
		if n != 1 {
			t.Fatal("Wrong length", n, p)
		}
		actual := buf.String()
		expected := CONTENTS[p.result : p.result+1]
		if actual != expected {
			t.Error("Contents wrong, expecting", expected, "got", actual, p)
		}
	}

	err = file.Close()
	if err != nil {
		t.Fatal(err)
	}
}

func TestObjectUpdate(t *testing.T) {
	err := c.ObjectUpdate(CONTAINER, OBJECT, m1.ObjectHeaders())
	if err != nil {
		t.Fatal(err)
	}
}

func checkTime(t *testing.T, when time.Time, low, high int) {
	dt := time.Now().Sub(when)
	if dt < time.Duration(low)*time.Second || dt > time.Duration(high)*time.Second {
		t.Errorf("Time is wrong: dt=%q, when=%q", dt, when)
	}
}

func TestObject(t *testing.T) {
	object, headers, err := c.Object(CONTAINER, OBJECT)
	if err != nil {
		t.Fatal(err)
	}
	compareMaps(t, headers.ObjectMetadata(), map[string]string{"hello": "1", "potato-salad": "2"})
	if object.Name != OBJECT || object.Bytes != CONTENT_SIZE || object.ContentType != "application/octet-stream" || object.Hash != CONTENT_MD5 || object.PseudoDirectory != false || object.SubDir != "" {
		t.Error("Bad object info", object)
	}
	checkTime(t, object.LastModified, -10, 10)
}

func TestObjectUpdate2(t *testing.T) {
	err := c.ObjectUpdate(CONTAINER, OBJECT, m2.ObjectHeaders())
	if err != nil {
		t.Fatal(err)
	}
	_, headers, err := c.Object(CONTAINER, OBJECT)
	if err != nil {
		t.Fatal(err)
	}
	//fmt.Println(headers, headers.ObjectMetadata())
	compareMaps(t, headers.ObjectMetadata(), map[string]string{"hello": "", "potato-salad": ""})
}

func TestContainers(t *testing.T) {
	containers, err := c.Containers(nil)
	if err != nil {
		t.Fatal(err)
	}
	ok := false
	for _, container := range containers {
		if container.Name == CONTAINER {
			ok = true
			// Container may or may not have the file contents in it
			// Swift updates may be behind
			if container.Count == 0 && container.Bytes == 0 {
				break
			}
			if container.Count == 1 && container.Bytes == CONTENT_SIZE {
				break
			}
			t.Errorf("Bad size of Container %q: %q", CONTAINER, container)
			break
		}
	}
	if !ok {
		t.Errorf("Didn't find container %q in listing %q", CONTAINER, containers)
	}
	//fmt.Println(containers)
}

func TestObjectNames(t *testing.T) {
	objects, err := c.ObjectNames(CONTAINER, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(objects) != 1 || objects[0] != OBJECT {
		t.Error("Incorrect listing", objects)
	}
	//fmt.Println(objects)
}

func TestObjectNamesAll(t *testing.T) {
	objects, err := c.ObjectNamesAll(CONTAINER, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(objects) != 1 || objects[0] != OBJECT {
		t.Error("Incorrect listing", objects)
	}
	//fmt.Println(objects)
}

func TestObjectNamesAllWithLimit(t *testing.T) {
	objects, err := c.ObjectNamesAll(CONTAINER, &swift.ObjectsOpts{Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(objects) != 1 || objects[0] != OBJECT {
		t.Error("Incorrect listing", objects)
	}
	//fmt.Println(objects)
}

func TestObjectsWalk(t *testing.T) {
	objects := make([]string, 0)
	err := c.ObjectsWalk(container, nil, func(opts *swift.ObjectsOpts) (interface{}, error) {
		newObjects, err := c.ObjectNames(CONTAINER, opts)
		if err == nil {
			objects = append(objects, newObjects...)
		}
		return newObjects, err
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(objects) != 1 || objects[0] != OBJECT {
		t.Error("Incorrect listing", objects)
	}
	//fmt.Println(objects)
}

func TestObjects(t *testing.T) {
	objects, err := c.Objects(CONTAINER, &swift.ObjectsOpts{Delimiter: '/'})
	if err != nil {
		t.Fatal(err)
	}
	if len(objects) != 1 {
		t.Fatal("Should only be 1 object")
	}
	object := objects[0]
	if object.Name != OBJECT || object.Bytes != CONTENT_SIZE || object.ContentType != "application/octet-stream" || object.Hash != CONTENT_MD5 || object.PseudoDirectory != false || object.SubDir != "" {
		t.Error("Bad object info", object)
	}
	checkTime(t, object.LastModified, -10, 10)
	// fmt.Println(objects)
}

func TestObjectsDirectory(t *testing.T) {
	err := c.ObjectPutString(CONTAINER, "directory", "", "application/directory")
	if err != nil {
		t.Fatal(err)
	}
	defer c.ObjectDelete(CONTAINER, "directory")

	// Look for the directory object and check we aren't confusing
	// it with a pseudo directory object
	objects, err := c.Objects(CONTAINER, &swift.ObjectsOpts{Delimiter: '/'})
	if err != nil {
		t.Fatal(err)
	}
	if len(objects) != 2 {
		t.Fatal("Should only be 2 objects")
	}
	found := false
	for i := range objects {
		object := objects[i]
		if object.Name == "directory" {
			found = true
			if object.Bytes != 0 || object.ContentType != "application/directory" || object.Hash != "d41d8cd98f00b204e9800998ecf8427e" || object.PseudoDirectory != false || object.SubDir != "" {
				t.Error("Bad object info", object)
			}
			checkTime(t, object.LastModified, -10, 10)
		}
	}
	if !found {
		t.Error("Didn't find directory object")
	}
	// fmt.Println(objects)
}

func TestObjectsPseudoDirectory(t *testing.T) {
	err := c.ObjectPutString(CONTAINER, "directory/puppy.jpg", "cute puppy", "")
	if err != nil {
		t.Fatal(err)
	}
	defer c.ObjectDelete(CONTAINER, "directory/puppy.jpg")

	// Look for the pseudo directory
	objects, err := c.Objects(CONTAINER, &swift.ObjectsOpts{Delimiter: '/'})
	if err != nil {
		t.Fatal(err)
	}
	if len(objects) != 2 {
		t.Fatal("Should only be 2 objects", objects)
	}
	found := false
	for i := range objects {
		object := objects[i]
		if object.Name == "directory/" {
			found = true
			if object.Bytes != 0 || object.ContentType != "application/directory" || object.Hash != "" || object.PseudoDirectory != true || object.SubDir != "directory/" && object.LastModified.IsZero() {
				t.Error("Bad object info", object)
			}
		}
	}
	if !found {
		t.Error("Didn't find directory object", objects)
	}

	// Look in the pseudo directory now
	objects, err = c.Objects(CONTAINER, &swift.ObjectsOpts{Delimiter: '/', Path: "directory/"})
	if err != nil {
		t.Fatal(err)
	}
	if len(objects) != 1 {
		t.Fatal("Should only be 1 object", objects)
	}
	object := objects[0]
	if object.Name != "directory/puppy.jpg" || object.Bytes != 10 || object.ContentType != "image/jpeg" || object.Hash != "87a12ea22fca7f54f0cefef1da535489" || object.PseudoDirectory != false || object.SubDir != "" {
		t.Error("Bad object info", object)
	}
	checkTime(t, object.LastModified, -10, 10)
	// fmt.Println(objects)
}

func TestObjectsAll(t *testing.T) {
	objects, err := c.ObjectsAll(CONTAINER, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(objects) != 1 || objects[0].Name != OBJECT {
		t.Error("Incorrect listing", objects)
	}
	//fmt.Println(objects)
}

func TestObjectsAllWithLimit(t *testing.T) {
	objects, err := c.ObjectsAll(CONTAINER, &swift.ObjectsOpts{Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(objects) != 1 || objects[0].Name != OBJECT {
		t.Error("Incorrect listing", objects)
	}
	//fmt.Println(objects)
}

func TestObjectNamesWithPath(t *testing.T) {
	objects, err := c.ObjectNames(CONTAINER, &swift.ObjectsOpts{Delimiter: '/', Path: ""})
	if err != nil {
		t.Fatal(err)
	}
	if len(objects) != 1 || objects[0] != OBJECT {
		t.Error("Bad listing with path", objects)
	}
	// fmt.Println(objects)
	objects, err = c.ObjectNames(CONTAINER, &swift.ObjectsOpts{Delimiter: '/', Path: "Downloads/"})
	if err != nil {
		t.Fatal(err)
	}
	if len(objects) != 0 {
		t.Error("Bad listing with path", objects)
	}
	// fmt.Println(objects)
}

func TestObjectCopy(t *testing.T) {
	_, err := c.ObjectCopy(CONTAINER, OBJECT, CONTAINER, OBJECT2, nil)
	if err != nil {
		t.Fatal(err)
	}
	err = c.ObjectDelete(CONTAINER, OBJECT2)
	if err != nil {
		t.Fatal(err)
	}
}

func TestObjectCopyWithMetadata(t *testing.T) {
	m := swift.Metadata{}
	m["copy-special-metadata"] = "hello"
	m["hello"] = "3"
	h := m.ObjectHeaders()
	h["Content-Type"] = "image/jpeg"
	_, err := c.ObjectCopy(CONTAINER, OBJECT, CONTAINER, OBJECT2, h)
	if err != nil {
		t.Fatal(err)
	}
	// Re-read the metadata to see if it is correct
	_, headers, err := c.Object(CONTAINER, OBJECT2)
	if err != nil {
		t.Fatal(err)
	}
	if headers["Content-Type"] != "image/jpeg" {
		t.Error("Didn't change content type")
	}
	compareMaps(t, headers.ObjectMetadata(), map[string]string{"hello": "3", "potato-salad": "", "copy-special-metadata": "hello"})
	err = c.ObjectDelete(CONTAINER, OBJECT2)
	if err != nil {
		t.Fatal(err)
	}
}

func TestObjectMove(t *testing.T) {
	err := c.ObjectMove(CONTAINER, OBJECT, CONTAINER, OBJECT2)
	if err != nil {
		t.Fatal(err)
	}
	testExistenceAfterDelete(t, CONTAINER, OBJECT)
	_, _, err = c.Object(CONTAINER, OBJECT2)
	if err != nil {
		t.Fatal(err)
	}

	err = c.ObjectMove(CONTAINER, OBJECT2, CONTAINER, OBJECT)
	if err != nil {
		t.Fatal(err)
	}
	testExistenceAfterDelete(t, CONTAINER, OBJECT2)
	_, headers, err := c.Object(CONTAINER, OBJECT)
	if err != nil {
		t.Fatal(err)
	}
	compareMaps(t, headers.ObjectMetadata(), map[string]string{"hello": "", "potato-salad": ""})
}

func TestObjectUpdateContentType(t *testing.T) {
	err := c.ObjectUpdateContentType(CONTAINER, OBJECT, "text/potato")
	if err != nil {
		t.Fatal(err)
	}
	// Re-read the metadata to see if it is correct
	_, headers, err := c.Object(CONTAINER, OBJECT)
	if err != nil {
		t.Fatal(err)
	}
	if headers["Content-Type"] != "text/potato" {
		t.Error("Didn't change content type")
	}
	compareMaps(t, headers.ObjectMetadata(), map[string]string{"hello": "", "potato-salad": ""})
}

func TestVersionContainerCreate(t *testing.T) {
	if err := c.VersionContainerCreate(CURRENT_CONTAINER, VERSIONS_CONTAINER); err != nil {
		if err == swift.Forbidden {
			t.Log("Server doesn't support Versions - skipping test")
			skipVersionTests = true
			return
		}
		t.Fatal(err)
	}
}

func TestVersionObjectAdd(t *testing.T) {
	if skipVersionTests {
		t.Log("Server doesn't support Versions - skipping test")
		return
	}
	// Version 1
	if err := c.ObjectPutString(CURRENT_CONTAINER, OBJECT, CONTENTS, ""); err != nil {
		t.Fatal(err)
	}
	if contents, err := c.ObjectGetString(CURRENT_CONTAINER, OBJECT); err != nil {
		t.Fatal(err)
	} else if contents != CONTENTS {
		t.Error("Contents wrong")
	}

	// Version 2
	if err := c.ObjectPutString(CURRENT_CONTAINER, OBJECT, CONTENTS2, ""); err != nil {
		t.Fatal(err)
	}
	if contents, err := c.ObjectGetString(CURRENT_CONTAINER, OBJECT); err != nil {
		t.Fatal(err)
	} else if contents != CONTENTS2 {
		t.Error("Contents wrong")
	}

	// Version 3
	if err := c.ObjectPutString(CURRENT_CONTAINER, OBJECT, CONTENTS2, ""); err != nil {
		t.Fatal(err)
	}
}

func TestVersionObjectList(t *testing.T) {
	if skipVersionTests {
		t.Log("Server doesn't support Versions - skipping test")
		return
	}
	list, err := c.VersionObjectList(VERSIONS_CONTAINER, OBJECT)
	if err != nil {
		t.Fatal(err)
	}

	if len(list) != 2 {
		t.Error("Version list should return 2 objects")
	}

	//fmt.Print(list)
}

func TestVersionObjectDelete(t *testing.T) {
	if skipVersionTests {
		t.Log("Server doesn't support Versions - skipping test")
		return
	}
	// Delete Version 3
	if err := c.ObjectDelete(CURRENT_CONTAINER, OBJECT); err != nil {
		t.Fatal(err)
	}

	// Delete Version 2
	if err := c.ObjectDelete(CURRENT_CONTAINER, OBJECT); err != nil {
		t.Fatal(err)
	}

	// Contents should be reverted to Version 1
	if contents, err := c.ObjectGetString(CURRENT_CONTAINER, OBJECT); err != nil {
		t.Fatal(err)
	} else if contents != CONTENTS {
		t.Error("Contents wrong")
	}
}

// cleanUpContainer deletes everything in the container and then the
// container.  It expects the container to be empty and if it wasn't
// it logs an error.
func cleanUpContainer(t *testing.T, container string) {
	objects, err := c.Objects(container, nil)
	if err != nil {
		t.Error(err, container)
	} else {
		if len(objects) != 0 {
			t.Error("Container not empty", container)
		}
		for _, object := range objects {
			t.Log("Deleting spurious", object.Name)
			err = c.ObjectDelete(container, object.Name)
			if err != nil {
				t.Error(err, container)
			}
		}
	}

	if err := c.ContainerDelete(container); err != nil {
		t.Error(err, container)
	}
}

func TestVersionDeleteContent(t *testing.T) {
	if skipVersionTests {
		t.Log("Server doesn't support Versions - skipping test")
	} else {
		// Delete Version 1
		if err := c.ObjectDelete(CURRENT_CONTAINER, OBJECT); err != nil {
			t.Fatal(err)
		}
	}
	cleanUpContainer(t, VERSIONS_CONTAINER)
	cleanUpContainer(t, CURRENT_CONTAINER)
}

// Check for non existence after delete
// May have to do it a few times to wait for swift to be consistent.
func testExistenceAfterDelete(t *testing.T, container, object string) {
	for i := 10; i <= 0; i-- {
		_, _, err := c.Object(container, object)
		if err == swift.ObjectNotFound {
			break
		}
		if i == 0 {
			t.Fatalf("Expecting object %q/%q not found not: err=%v", container, object, err)
		}
		time.Sleep(1 * time.Second)
	}
}

func TestObjectDelete(t *testing.T) {
	err := c.ObjectDelete(CONTAINER, OBJECT)
	if err != nil {
		t.Fatal(err)
	}
	testExistenceAfterDelete(t, CONTAINER, OBJECT)
	err = c.ObjectDelete(CONTAINER, OBJECT)
	if err != swift.ObjectNotFound {
		t.Fatal("Expecting Object not found", err)
	}
}

func TestBulkDelete(t *testing.T) {
	result, err := c.BulkDelete(CONTAINER, []string{OBJECT})
	if err == swift.Forbidden {
		t.Log("Server doesn't support BulkDelete - skipping test")
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	if result.NumberNotFound != 1 {
		t.Error("Expected 1, actual:", result.NumberNotFound)
	}
	if result.NumberDeleted != 0 {
		t.Error("Expected 0, actual:", result.NumberDeleted)
	}
	err = c.ObjectPutString(CONTAINER, OBJECT, CONTENTS, "")
	if err != nil {
		t.Fatal(err)
	}
	result, err = c.BulkDelete(CONTAINER, []string{OBJECT2, OBJECT})
	if err != nil {
		t.Fatal(err)
	}
	if result.NumberNotFound != 1 {
		t.Error("Expected 1, actual:", result.NumberNotFound)
	}
	if result.NumberDeleted != 1 {
		t.Error("Expected 1, actual:", result.NumberDeleted)
	}
	t.Log("Errors:", result.Errors)
}

func TestBulkUpload(t *testing.T) {
	buffer := new(bytes.Buffer)
	ds := tar.NewWriter(buffer)
	var files = []struct{ Name, Body string }{
		{OBJECT, CONTENTS},
		{OBJECT2, CONTENTS2},
	}
	for _, file := range files {
		hdr := &tar.Header{
			Name: file.Name,
			Size: int64(len(file.Body)),
		}
		if err := ds.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if _, err := ds.Write([]byte(file.Body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := ds.Close(); err != nil {
		t.Fatal(err)
	}

	result, err := c.BulkUpload(CONTAINER, buffer, swift.UploadTar, nil)
	if err == swift.Forbidden {
		t.Log("Server doesn't support BulkUpload - skipping test")
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	if result.NumberCreated != 2 {
		t.Error("Expected 2, actual:", result.NumberCreated)
	}
	t.Log("Errors:", result.Errors)

	_, _, err = c.Object(CONTAINER, OBJECT)
	if err != nil {
		t.Error("Expecting object to be found")
	}
	_, _, err = c.Object(CONTAINER, OBJECT2)
	if err != nil {
		t.Error("Expecting object to be found")
	}
	c.ObjectDelete(CONTAINER, OBJECT)
	c.ObjectDelete(CONTAINER, OBJECT2)
}

func TestObjectDifficultName(t *testing.T) {
	const name = `hello? sausage/êé/Hello, 世界/ " ' @ < > & ?/`
	err := c.ObjectPutString(CONTAINER, name, CONTENTS, "")
	if err != nil {
		t.Fatal(err)
	}
	objects, err := c.ObjectNamesAll(CONTAINER, nil)
	if err != nil {
		t.Error(err)
	}
	found := false
	for _, object := range objects {
		if object == name {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Couldn't find %q in listing %q", name, objects)
	}
	err = c.ObjectDelete(CONTAINER, name)
	if err != nil {
		t.Fatal(err)
	}
}

func TestTempUrl(t *testing.T) {
	err := c.ObjectPutBytes(CONTAINER, OBJECT, []byte(CONTENTS), "")
	if err != nil {
		t.Fatal(err)
	}

	m := swift.Metadata{}
	m["temp-url-key"] = SECRET_KEY
	err = c.AccountUpdate(m.AccountHeaders())
	if err != nil {
		t.Fatal(err)
	}

	expiresTime := time.Now().Add(20 * time.Minute)
	tempUrl := c.ObjectTempUrl(CONTAINER, OBJECT, SECRET_KEY, "GET", expiresTime)
	resp, err := http.Get(tempUrl)
	if err != nil {
		t.Fatal("Failed to retrieve file from temporary url")
	}
	if resp.StatusCode == 401 {
		t.Log("Server doesn't support tempurl")
	} else if resp.StatusCode != 200 {
		t.Fatal("HTTP Error retrieving file from temporary url", resp.StatusCode)
	} else {
		if content, err := ioutil.ReadAll(resp.Body); err != nil || string(content) != CONTENTS {
			t.Error("Bad content", err)
		}

		resp, err := http.Post(tempUrl, "image/jpeg", bytes.NewReader([]byte(CONTENTS)))
		if err != nil {
			t.Fatal("Failed to retrieve file from temporary url")
		}
		if resp.StatusCode != 401 {
			t.Fatal("Expecting server to forbid access to object")
		}
	}

	resp.Body.Close()
	err = c.ObjectDelete(CONTAINER, OBJECT)
	if err != nil {
		t.Fatal(err)
	}
}

func TestQueryInfo(t *testing.T) {
	infos, err := c.QueryInfo()
	if err != nil {
		t.Log("Server doesn't support querying info")
		return
	}
	if _, ok := infos["swift"]; !ok {
		t.Fatal("No 'swift' section found in configuration")
	}
}

func TestContainerDelete(t *testing.T) {
	err := c.ContainerDelete(CONTAINER)
	if err != nil {
		t.Fatal(err)
	}
	err = c.ContainerDelete(CONTAINER)
	if err != swift.ContainerNotFound {
		t.Fatal("Expecting container not found", err)
	}
	_, _, err = c.Container(CONTAINER)
	if err != swift.ContainerNotFound {
		t.Fatal("Expecting container not found", err)
	}
}

func TestUnAuthenticate(t *testing.T) {
	c.UnAuthenticate()
	if c.Authenticated() {
		t.Fatal("Shouldn't be authenticated")
	}
	// Test re-authenticate
	err := c.Authenticate()
	if err != nil {
		t.Fatal("ReAuth failed", err)
	}
	if !c.Authenticated() {
		t.Fatal("Not authenticated")
	}
}
