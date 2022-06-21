package ontap

import (
	"context"
	"crypto/md5"
	"crypto/tls"
	"fmt"
	"hash"
	"io"
	"net/http"
	"os"
	"path"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3iface"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/cenkalti/backoff/v4"
	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/restic"
)

const ProtocolScheme = "ontaps3"
const defaultLayout = "default"

// wrapReader wraps an io.ReadCloser to run an additional function on Close.
type wrapReader struct {
	io.ReadCloser
	f func()
}

func (wr wrapReader) Close() error {
	err := wr.ReadCloser.Close()
	wr.f()
	return err
}

type Backend struct {
	client s3iface.S3API
	sem    *backend.Semaphore
	cfg    Config
	backend.Layout
}

// Hasher may return a hash function for calculating a content hash for the backend
func (be *Backend) Hasher() hash.Hash {
	return md5.New()
}

func (be *Backend) Join(s ...string) string {
	return path.Join(s...)
}

type fileInfo struct {
	name    string
	size    int64
	mode    os.FileMode
	modTime time.Time
	isDir   bool
}

func (fi fileInfo) Name() string       { return fi.name }    // base name of the file
func (fi fileInfo) Size() int64        { return fi.size }    // length in bytes for regular files; system-dependent for others
func (fi fileInfo) Mode() os.FileMode  { return fi.mode }    // file mode bits
func (fi fileInfo) ModTime() time.Time { return fi.modTime } // modification time
func (fi fileInfo) IsDir() bool        { return fi.isDir }   // abbreviation for Mode().IsDir()
func (fi fileInfo) Sys() interface{}   { return nil }        // underlying data source (can return nil)

// ReadDir returns the entries for a directory.
func (be *Backend) ReadDir(ctx context.Context, dir string) (list []os.FileInfo, err error) {
	debug.Log("ReadDir(%v)", dir)

	// make sure dir ends with a slash
	if !strings.HasSuffix(dir, "/") {
		dir += "/"
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	output, err := be.client.ListObjectsV2WithContext(ctx, &s3.ListObjectsV2Input{
		Prefix: &dir,
	})
	if err != nil {
		return nil, err
	}

	for _, obj := range output.Contents {
		if *obj.Key == "" {
			continue
		}

		name := strings.TrimPrefix(*obj.Key, dir)
		// Sometimes s3 returns an entry for the dir itself. Ignore it.
		if name == "" {
			continue
		}
		entry := fileInfo{
			name:    name,
			size:    *obj.Size,
			modTime: *obj.LastModified,
		}

		if strings.HasSuffix(name, "/") {
			entry.isDir = true
			entry.mode = os.ModeDir | 0755
			entry.name = name[:len(name)-1]
		} else {
			entry.mode = 0644
		}

		list = append(list, entry)
	}

	return list, nil
}

func (be *Backend) Location() string {
	return be.Join(*be.cfg.Bucket, be.cfg.Prefix)
}

// Path returns the path in the bucket that is used for this backend.
func (be *Backend) Path() string {
	return be.cfg.Prefix
}

// Test tests that a file exists
func (be *Backend) Test(ctx context.Context, h restic.Handle) (bool, error) {
	debug.Log("Test: h=%v\n", h)

	found := false
	objName := be.Filename(h)
	params := s3.HeadObjectInput{
		Bucket: be.cfg.Bucket,
		Key:    &objName,
	}

	be.sem.GetToken()
	output, err := be.client.HeadObjectWithContext(ctx, &params)
	be.sem.ReleaseToken()
	debug.Log("HeadObject result: %v, %v\n", output, err)

	if err == nil {
		found = true
	}

	return found, nil
}

// Remove removes the blob with the given name and type.
func (be *Backend) Remove(ctx context.Context, h restic.Handle) error {
	input := &s3.DeleteObjectInput{
		Bucket: be.cfg.Bucket,
		Key:    aws.String(be.Filename(h)),
	}

	be.sem.GetToken()
	output, err := be.client.DeleteObjectWithContext(ctx, input)
	be.sem.ReleaseToken()
	debug.Log("Delete object output: %v\n", output)

	return err
}

// Close is not supported
func (be *Backend) Close() error {
	return nil
}

// Save will write data to the bucket
func (be *Backend) Save(ctx context.Context, h restic.Handle, rd restic.RewindReader) error {
	debug.Log("Save %v", h)
	debug.Log("PutObject %v\n", h)

	if err := h.Valid(); err != nil {
		return backoff.Permanent(err)
	}

	objName := be.Filename(h)
	debug.Log("name: %v", objName)

	svc := s3manager.NewUploaderWithClient(be.client)
	input := &s3manager.UploadInput{
		Bucket: be.cfg.Bucket,
		Key:    &objName,
		Body:   rd,
	}

	be.sem.GetToken()
	defer be.sem.ReleaseToken()

	debug.Log("PutObject(%v, %v, %v)", be.cfg.Bucket, objName, rd.Length())
	output, err := svc.UploadWithContext(ctx, input)

	debug.Log("PutObject output: %v %v\n", output, err)
	if err != nil {
		return err
	}

	return nil
}

func (be *Backend) Load(ctx context.Context, h restic.Handle, length int, offset int64, fn func(rd io.Reader) error) error {
	return backend.DefaultLoad(ctx, h, length, offset, be.openReader, fn)
}

func (be *Backend) openReader(ctx context.Context, h restic.Handle, length int, offset int64) (io.ReadCloser, error) {
	debug.Log("Load %v, length %v, offset %v from %v", h, length, offset, be.Filename(h))
	if err := h.Valid(); err != nil {
		return nil, backoff.Permanent(err)
	}

	objName := be.Filename(h)

	if length < 0 || offset < 0 {
		return nil, errors.New("length and offset cannot be negative")
	}

	var bytesRange *string
	if length > 0 {
		s := fmt.Sprintf("bytes=%v-%v", offset, offset+int64(length)-1)
		bytesRange = &s
	} else if offset > 0 {
		s := fmt.Sprintf("bytes=%v-%v", offset, 0)
		bytesRange = &s
	}
	debug.Log("Range: %v", bytesRange)

	be.sem.GetToken()
	output, err := be.client.GetObjectWithContext(ctx, &s3.GetObjectInput{
		Bucket: be.cfg.Bucket,
		Key:    &objName,
		Range:  bytesRange,
	})

	if err != nil {
		be.sem.ReleaseToken()
		return nil, err
	}

	closeRd := wrapReader{
		ReadCloser: output.Body,
		f: func() {
			debug.Log("Close()")
			be.sem.ReleaseToken()
		},
	}

	return closeRd, err
}

// Stat returns information about a blob.
func (be *Backend) Stat(ctx context.Context, h restic.Handle) (bi restic.FileInfo, err error) {
	debug.Log("%v", h)

	objName := be.Filename(h)

	input := s3.GetObjectInput{
		Bucket: be.cfg.Bucket,
		Key:    &objName,
	}

	be.sem.GetToken()
	output, err := be.client.GetObjectWithContext(ctx, &input)
	if err != nil {
		debug.Log("GetObject() err %v", err)
		be.sem.ReleaseToken()
		return restic.FileInfo{}, errors.Wrap(err, "client.GetObject")
	}

	defer func() {
		e := output.Body.Close()
		be.sem.ReleaseToken()
		if err == nil {
			err = errors.Wrap(e, "Close")
		}
	}()

	return restic.FileInfo{Size: *output.ContentLength, Name: h.Name}, nil
}

// List runs fn for each file in the backend which has the type t. When an
// error occurs (or fn returns an error), List stops and returns it.
func (be *Backend) List(ctx context.Context, t restic.FileType, fn func(restic.FileInfo) error) error {
	debug.Log("listing %v", t)

	prefix, _ := be.Basedir(t)

	// make sure prefix ends with a slash
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	input := s3.ListObjectsV2Input{
		Bucket: be.cfg.Bucket,
		Prefix: &prefix,
	}

	debug.Log("using ListObjectsV2(%v)", input)

	output, err := be.client.ListObjectsV2WithContext(ctx, &input)
	if err != nil {
		return err
	}

	for _, obj := range output.Contents {
		m := strings.TrimPrefix(*obj.Key, prefix)
		if m == "" {
			continue
		}

		fi := restic.FileInfo{
			Name: path.Base(m),
			Size: *obj.Size,
		}

		err = fn(fi)
		if err != nil {
			return err
		}
	}

	return nil
}

// IsNotExist returns true if the error is caused by a not existing file.
func (be *Backend) IsNotExist(err error) bool {
	debug.Log("IsNotExist(%T, %#v)", err, err)
	if os.IsNotExist(errors.Cause(err)) {
		return true
	}

	debug.Log("Unknown error %T: %v", err, err)
	return false
}

// Remove keys for a specified backend type.
func (be *Backend) removeKeys(ctx context.Context, t restic.FileType) error {
	return be.List(ctx, t, func(fi restic.FileInfo) error {
		return be.Remove(ctx, restic.Handle{Type: t, Name: fi.Name})
	})
}

// Delete removes all restic keys in the bucket
func (be *Backend) Delete(ctx context.Context) error {
	allTypes := []restic.FileType{
		restic.PackFile,
		restic.KeyFile,
		restic.KeysFile,
		restic.LockFile,
		restic.SnapshotFile,
		restic.IndexFile,
	}

	for _, t := range allTypes {
		err := be.removeKeys(ctx, t)
		if err != nil {
			return err
		}
	}

	return be.Remove(ctx, restic.Handle{Type: restic.ConfigFile})
}

func Open(ctx context.Context, config Config) (*Backend, error) {
	debug.Log("Open: Generating config")

	envVar := os.Getenv("FORCE_CERT_VALIDATION")
	envVar = strings.ToLower(strings.TrimSpace(envVar))
	var skipValidation bool
	if "true" == envVar {
		skipValidation = false
	} else {
		skipValidation = true
	}

	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: skipValidation},
	}
	httpClient := &http.Client{Transport: tr}

	newConfig := aws.Config{
		Endpoint:         aws.String(config.GetAPIURL()),
		DisableSSL:       aws.Bool(true),
		S3ForcePathStyle: aws.Bool(true),
		Region:           aws.String("ontap"),
		HTTPClient:       httpClient,
	}

	newSession, err := session.NewSession(&newConfig)
	if err != nil {
		return nil, err
	}

	client := s3.New(newSession)

	sem, err := backend.NewSemaphore(config.Connections)
	if err != nil {
		return nil, err
	}

	newBackend := NewBackend(client, sem, config)

	layout, err := backend.ParseLayout(ctx, newBackend, "default", defaultLayout, config.Prefix)
	if err != nil {
		return nil, err
	}

	newBackend.Layout = layout

	return newBackend, nil
}

func NewBackend(client s3iface.S3API, sem *backend.Semaphore, config Config) *Backend {
	return &Backend{
		client: client,
		sem:    sem,
		cfg:    config,
	}
}
