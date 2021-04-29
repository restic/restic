package ontap_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3iface"
	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/backend/ontap"
	"github.com/restic/restic/internal/backend/test"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
	"github.com/stretchr/testify/assert"
)

type OntapTestConfig struct {
	Enabled bool
	Config  string
}

var testParams OntapTestConfig

func init() {
	enabled := false
	enabledString := os.Getenv("RESTIC_TEST_ONTAP_ENABLED")
	if strings.TrimSpace(strings.ToLower(enabledString)) == "true" {
		enabled = true
	}
	testParams = OntapTestConfig{
		Enabled: enabled,
		Config:  os.Getenv("RESTIC_TEST_ONTAP_CONFIG"),
	}

}

func newOntapTestSuite(ctx context.Context, t testing.TB) *test.Suite {

	return &test.Suite{
		NewConfig: func() (interface{}, error) {
			cfg, err := ontap.ParseConfig(testParams.Config)
			if err != nil {
				t.Fatalf("Error generating backend config: %v", err)
				return nil, err
			}
			return cfg, nil
		},

		Create: func(config interface{}) (restic.Backend, error) {
			cfg := config.(ontap.Config)

			be, err := ontap.Open(ctx, cfg)
			if err != nil {
				return nil, err
			}

			h := restic.Handle{
				Type: restic.ConfigFile,
				Name: "config",
			}

			fileExists, err := be.Test(ctx, h)
			if err != nil {
				return nil, err
			}

			if fileExists {
				return nil, errors.New(fmt.Sprintf("%v already exists", restic.ConfigFile))
			}

			return be, nil
		},

		Open: func(config interface{}) (restic.Backend, error) {
			cfg := config.(ontap.Config)

			be, err := ontap.Open(ctx, cfg)
			if err != nil {
				return nil, err
			}

			return be, nil
		},

		// CleanupFn removes data created during the tests.
		Cleanup: func(config interface{}) error {
			cfg := config.(ontap.Config)

			be, err := ontap.Open(ctx, cfg)
			if err != nil {
				return err
			}

			return be.Delete(ctx)
		},
	}
}

func TestBackendOntap(t *testing.T) {
	if !testParams.Enabled {
		t.Skip("Ontap Backend integration test disabled")
	}

	defer func() {
		if t.Skipped() {
			rtest.SkipDisallowed(t, "restic/backend/ontap.TestBackendOntap")
		}
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	t.Logf("run tests")
	newOntapTestSuite(ctx, t).RunTests(t)
}

func BenchmarkBackendOntap(t *testing.B) {
	if !testParams.Enabled {
		t.Skip("Ontap Backend benchmark test disabled")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	t.Logf("run tests")
	newOntapTestSuite(ctx, t).RunBenchmarks(t)
}

type readCloseTester struct {
	io.Reader
	Err error
}

func (r readCloseTester) Close() error {
	return r.Err
}

type mockS3Api struct {
	s3iface.S3API
	Input interface{}
	Body  readCloseTester
	Err   error
}

func (client *mockS3Api) ListObjectsV2WithContext(_ aws.Context, input *s3.ListObjectsV2Input, _ ...request.Option) (*s3.ListObjectsV2Output, error) {
	client.Input = input
	if client.Err != nil {
		return nil, client.Err
	}

	fakeTime := time.Now()
	output := &s3.ListObjectsV2Output{
		Contents: []*s3.Object{
			{Key: aws.String(""), LastModified: &fakeTime},
			{Key: input.Prefix, LastModified: &fakeTime},
			{Key: aws.String(*input.Prefix + "foofile"), Size: aws.Int64(1), LastModified: &fakeTime},
			{Key: aws.String(*input.Prefix + "foosubdir/"), Size: aws.Int64(2), LastModified: &fakeTime},
			{Key: aws.String(*input.Prefix + *input.Prefix), Size: aws.Int64(3), LastModified: &fakeTime},
		},
	}

	return output, nil
}

func (client *mockS3Api) HeadObjectWithContext(_ context.Context, _ *s3.HeadObjectInput, _ ...request.Option) (*s3.HeadObjectOutput, error) {
	if client.Err != nil {
		return nil, client.Err
	}
	return &s3.HeadObjectOutput{}, nil
}

func (client *mockS3Api) DeleteObjectWithContext(_ context.Context, _ *s3.DeleteObjectInput, _ ...request.Option) (*s3.DeleteObjectOutput, error) {
	if client.Err != nil {
		return nil, client.Err
	}
	return &s3.DeleteObjectOutput{}, nil
}

func (client *mockS3Api) PutObjectRequest(input *s3.PutObjectInput) (*request.Request, *s3.PutObjectOutput) {
	client.Input = input
	return &request.Request{HTTPRequest: &http.Request{Header: http.Header{}, URL: &url.URL{}}}, &s3.PutObjectOutput{}
}

func (client *mockS3Api) GetObjectWithContext(_ aws.Context, input *s3.GetObjectInput, _ ...request.Option) (*s3.GetObjectOutput, error) {
	if client.Err != nil {
		return nil, client.Err
	}
	client.Input = input
	var length int64 = 101
	return &s3.GetObjectOutput{Body: client.Body, ContentLength: &length}, nil
}

func makeBackend() (*ontap.Backend, *mockS3Api) {
	client := &mockS3Api{}
	sem, _ := backend.NewSemaphore(1)
	config := ontap.Config{Bucket: aws.String("foobucket"), Prefix: "fooprefix"}
	be := ontap.NewBackend(client, sem, config)
	layout, err := backend.ParseLayout(context.TODO(), be, "default", "default", config.Prefix)
	if err != nil {
		panic(err)
	}
	be.Layout = layout
	return be, client
}

func makeHandle() restic.Handle {
	return restic.Handle{Type: restic.ConfigFile, Name: "config"}
}

type fakeFileInfo struct {
	name  string
	size  int64
	isDir bool
	mode  os.FileMode
}

func TestReadDir(t *testing.T) {
	be, client := makeBackend()
	expectedList := []fakeFileInfo{
		{"foofile", 1, false, 0644},
		{"foosubdir", 2, true, os.ModeDir | 0755},
		{"foodir", 3, true, os.ModeDir | 0755},
	}
	list, err := be.ReadDir(context.TODO(), "foodir")

	assert.Nil(t, err)
	assert.Equal(t, len(list), 3)

	input := client.Input.(*s3.ListObjectsV2Input)
	assert.Equal(t, *input.Prefix, "foodir/")

	for i := range list {
		assert.Equal(t, expectedList[i].name, list[i].Name())
		assert.Equal(t, expectedList[i].size, list[i].Size())
		assert.Equal(t, expectedList[i].isDir, list[i].IsDir())
		assert.Equal(t, expectedList[i].mode, list[i].Mode())
		assert.NotNil(t, list[i].ModTime())
	}
}

func TestReadDirError(t *testing.T) {
	be, client := makeBackend()
	fakeError := errors.New("fake error")
	client.Err = fakeError
	_, err := be.ReadDir(context.TODO(), "foodir")
	assert.Equal(t, fakeError, err)
}

func TestLocation(t *testing.T) {
	be, _ := makeBackend()
	assert.Equal(t, "foobucket/fooprefix", be.Location())
}

func TestPath(t *testing.T) {
	be, _ := makeBackend()
	assert.Equal(t, "fooprefix", be.Path())
}

func TestFileExists(t *testing.T) {
	be, _ := makeBackend()
	handle := makeHandle()
	fileExists, err := be.Test(context.TODO(), handle)
	assert.Nil(t, err)
	assert.Equal(t, true, fileExists)
}

func TestFileDoesNotExist(t *testing.T) {
	be, client := makeBackend()
	handle := makeHandle()
	client.Err = errors.New("file not found")
	fileExists, err := be.Test(context.TODO(), handle)
	assert.Nil(t, err)
	assert.Equal(t, false, fileExists)
}

func TestRemove(t *testing.T) {
	be, _ := makeBackend()
	handle := makeHandle()
	err := be.Remove(context.TODO(), handle)
	assert.Nil(t, err)
}

func TestRemoveError(t *testing.T) {
	be, client := makeBackend()
	handle := makeHandle()
	client.Err = errors.New("failed delete")
	err := be.Remove(context.TODO(), handle)
	assert.Equal(t, client.Err, err)
}

func TestSave(t *testing.T) {
	be, client := makeBackend()
	handle := makeHandle()
	reader := bytes.NewReader([]byte("read me"))
	fileReader, err := restic.NewFileReader(reader)
	if err != nil {
		t.Fatal(err)
	}
	err = be.Save(context.TODO(), handle, fileReader)
	input := client.Input.(*s3.PutObjectInput)

	assert.Nil(t, err)
	assert.Equal(t, "foobucket", *input.Bucket)
	assert.Equal(t, "fooprefix/config", *input.Key)
	assert.NotNil(t, input.Body)
}

func TestLoad(t *testing.T) {
	be, client := makeBackend()
	handle := makeHandle()
	err := be.Load(context.TODO(), handle, 5, 10, func(rd io.Reader) error {
		return nil
	})
	assert.Nil(t, err)

	input := client.Input.(*s3.GetObjectInput)

	assert.Equal(t, "foobucket", *input.Bucket)
	assert.Equal(t, "fooprefix/config", *input.Key)
	assert.Equal(t, "bytes=10-14", *input.Range)
}

func TestLoadClientError(t *testing.T) {
	be, client := makeBackend()
	handle := makeHandle()
	client.Err = errors.New("uh oh 1")
	err := be.Load(context.TODO(), handle, 0, 0, func(rd io.Reader) error {
		return nil
	})
	assert.Equal(t, client.Err, err)
}

func TestLoadReadError(t *testing.T) {
	be, _ := makeBackend()
	handle := makeHandle()
	expectErr := errors.New("uh oh 2")
	err := be.Load(context.TODO(), handle, 0, 0, func(rd io.Reader) error {
		return expectErr
	})
	assert.Equal(t, expectErr, err)
}

func TestLoadRangeError(t *testing.T) {
	be, _ := makeBackend()
	handle := makeHandle()
	err := be.Load(context.TODO(), handle, -1, 0, func(rd io.Reader) error {
		return nil
	})
	assert.Error(t, err)
	err = be.Load(context.TODO(), handle, 0, -1, func(rd io.Reader) error {
		return nil
	})
	assert.Error(t, err)
}

func TestStat(t *testing.T) {
	be, _ := makeBackend()
	handle := makeHandle()
	info, err := be.Stat(context.TODO(), handle)
	assert.Nil(t, err)
	assert.Equal(t, restic.FileInfo{Size: 101, Name: "config"}, info)
}

func TestStatFileError(t *testing.T) {
	be, client := makeBackend()
	handle := makeHandle()
	client.Err = errors.New("file error")
	_, err := be.Stat(context.TODO(), handle)
	assert.Error(t, err)
}

func TestStatCloseError(t *testing.T) {
	be, client := makeBackend()
	handle := makeHandle()
	client.Body.Err = errors.New("close error")
	_, err := be.Stat(context.TODO(), handle)
	assert.Error(t, err)
}

func TestList(t *testing.T) {
	expectedList := []restic.FileInfo{
		{Name: "foofile", Size: 1},
		{Name: "foosubdir", Size: 2},
		{Name: "fooprefix", Size: 3},
	}
	callCount := 0

	be, _ := makeBackend()
	err := be.List(context.TODO(), restic.ConfigFile, func(info restic.FileInfo) error {
		expectInfo := expectedList[callCount]
		assert.Equal(t, expectInfo, info)
		callCount++
		return nil
	})
	assert.Nil(t, err)
	assert.Equal(t, 3, callCount)
}

func TestListError(t *testing.T) {
	be, client := makeBackend()
	client.Err = errors.New("call failed")
	err := be.List(context.TODO(), restic.ConfigFile, func(info restic.FileInfo) error {
		return nil
	})
	assert.Equal(t, client.Err, err)
}

func TestListHandlerError(t *testing.T) {
	be, _ := makeBackend()
	expectErr := errors.New("call failed")
	err := be.List(context.TODO(), restic.ConfigFile, func(info restic.FileInfo) error {
		return expectErr
	})
	assert.Equal(t, expectErr, err)
}

func TestDelete(t *testing.T) {
	be, _ := makeBackend()
	err := be.Delete(context.TODO())
	assert.Nil(t, err)
}

func TestDeleteError(t *testing.T) {
	be, client := makeBackend()
	client.Err = errors.New("failed delete")
	err := be.Delete(context.TODO())
	assert.Equal(t, client.Err, err)
}

func TestOpen(t *testing.T) {
	cfg, err := ontap.ParseConfig("ontaps3:address/bucket")
	assert.Nil(t, err)
	config := cfg.(ontap.Config)
	be, err := ontap.Open(context.TODO(), config)
	assert.Nil(t, err)
	assert.NotNil(t, be)
}

func TestParseConfig(t *testing.T) {
	var connections uint = 5
	cfg, err := ontap.ParseConfig("ontaps3:address/bucket/prefix")
	assert.Nil(t, err)
	config := cfg.(ontap.Config)
	assert.Equal(t, "prefix", config.Prefix)
	assert.Equal(t, "bucket", *config.Bucket)
	assert.Equal(t, connections, config.Connections)
	assert.Equal(t, "https://address", config.GetAPIURL())
}

func TestParseConfigNoPrefix(t *testing.T) {
	cfg, err := ontap.ParseConfig("ontaps3:address/bucket")
	assert.Nil(t, err)
	config := cfg.(ontap.Config)
	assert.Equal(t, ".", config.Prefix)
	cfg, err = ontap.ParseConfig("ontaps3:address/bucket/")
	assert.Nil(t, err)
	config = cfg.(ontap.Config)
	assert.Equal(t, ".", config.Prefix)
}

func TestParseConfigHttpAddress(t *testing.T) {
	cfg, err := ontap.ParseConfig("ontaps3:http://address/bucket/prefix")
	assert.Nil(t, err)
	config := cfg.(ontap.Config)
	assert.Equal(t, "http://address", config.GetAPIURL())
}

func TestParseConfigHttpsAddresses(t *testing.T) {
	cfg, err := ontap.ParseConfig("ontaps3:https://address/bucket/prefix")
	assert.Nil(t, err)
	config := cfg.(ontap.Config)
	assert.Equal(t, "https://address", config.GetAPIURL())
}

func TestParseConfigPrefixError(t *testing.T) {
	_, err := ontap.ParseConfig("ntap:address/bucket/prefix")
	assert.Error(t, err)
}

func TestParseConfigUrlError(t *testing.T) {
	_, err := ontap.ParseConfig("ontaps3:`/bucket/prefix")
	assert.Error(t, err)
	_, err = ontap.ParseConfig("ontaps3:")
	assert.Error(t, err)
	_, err = ontap.ParseConfig("ontaps3:/")
	assert.Error(t, err)
}

func TestParseConfigBucketNameError(t *testing.T) {
	_, err := ontap.ParseConfig("ontaps3:address")
	assert.Error(t, err)
	_, err = ontap.ParseConfig("ontaps3:address/")
	assert.Error(t, err)
}
