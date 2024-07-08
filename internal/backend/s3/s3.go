package s3

import (
	"context"
	"fmt"
	"hash"
	"io"
	"net/http"
	"os"
	"path"
	"strings"
	"time"

	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/backend/layout"
	"github.com/restic/restic/internal/backend/location"
	"github.com/restic/restic/internal/backend/util"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/feature"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// Backend stores data on an S3 endpoint.
type Backend struct {
	client *minio.Client
	cfg    Config
	layout.Layout
}

// make sure that *Backend implements backend.Backend
var _ backend.Backend = &Backend{}

func NewFactory() location.Factory {
	return location.NewHTTPBackendFactory("s3", ParseConfig, location.NoPassword, Create, Open)
}

const defaultLayout = "default"

func open(ctx context.Context, cfg Config, rt http.RoundTripper) (*Backend, error) {
	debug.Log("open, config %#v", cfg)

	if cfg.KeyID == "" && cfg.Secret.String() != "" {
		return nil, errors.Fatalf("unable to open S3 backend: Key ID ($AWS_ACCESS_KEY_ID) is empty")
	} else if cfg.KeyID != "" && cfg.Secret.String() == "" {
		return nil, errors.Fatalf("unable to open S3 backend: Secret ($AWS_SECRET_ACCESS_KEY) is empty")
	}

	if cfg.MaxRetries > 0 {
		minio.MaxRetry = int(cfg.MaxRetries)
	}

	creds, err := getCredentials(cfg, rt)
	if err != nil {
		return nil, errors.Wrap(err, "s3.getCredentials")
	}

	options := &minio.Options{
		Creds:     creds,
		Secure:    !cfg.UseHTTP,
		Region:    cfg.Region,
		Transport: rt,
	}

	switch strings.ToLower(cfg.BucketLookup) {
	case "", "auto":
		options.BucketLookup = minio.BucketLookupAuto
	case "dns":
		options.BucketLookup = minio.BucketLookupDNS
	case "path":
		options.BucketLookup = minio.BucketLookupPath
	default:
		return nil, fmt.Errorf(`bad bucket-lookup style %q must be "auto", "path" or "dns"`, cfg.BucketLookup)
	}

	client, err := minio.New(cfg.Endpoint, options)
	if err != nil {
		return nil, errors.Wrap(err, "minio.New")
	}

	be := &Backend{
		client: client,
		cfg:    cfg,
	}

	l, err := layout.ParseLayout(ctx, be, cfg.Layout, defaultLayout, cfg.Prefix)
	if err != nil {
		return nil, err
	}

	be.Layout = l

	return be, nil
}

// getCredentials -- runs through the various credential types and returns the first one that works.
// additionally if the user has specified a role to assume, it will do that as well.
func getCredentials(cfg Config, tr http.RoundTripper) (*credentials.Credentials, error) {
	if cfg.UnsafeAnonymousAuth {
		return credentials.New(&credentials.Static{}), nil
	}

	// Chains all credential types, in the following order:
	// 	- Static credentials provided by user
	//	- AWS env vars (i.e. AWS_ACCESS_KEY_ID)
	//  - Minio env vars (i.e. MINIO_ACCESS_KEY)
	//  - AWS creds file (i.e. AWS_SHARED_CREDENTIALS_FILE or ~/.aws/credentials)
	//  - Minio creds file (i.e. MINIO_SHARED_CREDENTIALS_FILE or ~/.mc/config.json)
	//  - IAM profile based credentials. (performs an HTTP
	//    call to a pre-defined endpoint, only valid inside
	//    configured ec2 instances)
	creds := credentials.NewChainCredentials([]credentials.Provider{
		&credentials.EnvAWS{},
		&credentials.Static{
			Value: credentials.Value{
				AccessKeyID:     cfg.KeyID,
				SecretAccessKey: cfg.Secret.Unwrap(),
			},
		},
		&credentials.EnvMinio{},
		&credentials.FileAWSCredentials{},
		&credentials.FileMinioClient{},
		&credentials.IAM{
			Client: &http.Client{
				Transport: tr,
			},
		},
	})

	c, err := creds.Get()
	if err != nil {
		return nil, errors.Wrap(err, "creds.Get")
	}

	if c.SignerType == credentials.SignatureAnonymous {
		// Fail if no credentials were found to prevent repeated attempts to (unsuccessfully) retrieve new credentials.
		// The first attempt still has to timeout which slows down restic usage considerably. Thus, migrate towards forcing
		// users to explicitly decide between authenticated and anonymous access.
		if feature.Flag.Enabled(feature.ExplicitS3AnonymousAuth) {
			return nil, fmt.Errorf("no credentials found. Use `-o s3.unsafe-anonymous-auth=true` for anonymous authentication")
		}

		debug.Log("using anonymous access for %#v", cfg.Endpoint)
		creds = credentials.New(&credentials.Static{})
	}

	roleArn := os.Getenv("RESTIC_AWS_ASSUME_ROLE_ARN")
	if roleArn != "" {
		// use the region provided by the configuration by default
		awsRegion := cfg.Region
		// allow the region to be overridden if for some reason it is required
		if os.Getenv("RESTIC_AWS_ASSUME_ROLE_REGION") != "" {
			awsRegion = os.Getenv("RESTIC_AWS_ASSUME_ROLE_REGION")
		}

		sessionName := os.Getenv("RESTIC_AWS_ASSUME_ROLE_SESSION_NAME")
		externalID := os.Getenv("RESTIC_AWS_ASSUME_ROLE_EXTERNAL_ID")
		policy := os.Getenv("RESTIC_AWS_ASSUME_ROLE_POLICY")
		stsEndpoint := os.Getenv("RESTIC_AWS_ASSUME_ROLE_STS_ENDPOINT")

		if stsEndpoint == "" {
			if awsRegion != "" {
				if strings.HasPrefix(awsRegion, "cn-") {
					stsEndpoint = "https://sts." + awsRegion + ".amazonaws.com.cn"
				} else {
					stsEndpoint = "https://sts." + awsRegion + ".amazonaws.com"
				}
			} else {
				stsEndpoint = "https://sts.amazonaws.com"
			}
		}

		opts := credentials.STSAssumeRoleOptions{
			RoleARN:         roleArn,
			AccessKey:       c.AccessKeyID,
			SecretKey:       c.SecretAccessKey,
			SessionToken:    c.SessionToken,
			RoleSessionName: sessionName,
			ExternalID:      externalID,
			Policy:          policy,
			Location:        awsRegion,
		}

		creds, err = credentials.NewSTSAssumeRole(stsEndpoint, opts)
		if err != nil {
			return nil, errors.Wrap(err, "creds.AssumeRole")
		}
	}

	return creds, nil
}

// Open opens the S3 backend at bucket and region. The bucket is created if it
// does not exist yet.
func Open(ctx context.Context, cfg Config, rt http.RoundTripper) (backend.Backend, error) {
	return open(ctx, cfg, rt)
}

// Create opens the S3 backend at bucket and region and creates the bucket if
// it does not exist yet.
func Create(ctx context.Context, cfg Config, rt http.RoundTripper) (backend.Backend, error) {
	be, err := open(ctx, cfg, rt)
	if err != nil {
		return nil, errors.Wrap(err, "open")
	}
	found, err := be.client.BucketExists(ctx, cfg.Bucket)

	if err != nil && isAccessDenied(err) {
		err = nil
		found = true
	}

	if err != nil {
		debug.Log("BucketExists(%v) returned err %v", cfg.Bucket, err)
		return nil, errors.Wrap(err, "client.BucketExists")
	}

	if !found {
		// create new bucket with default ACL in default region
		err = be.client.MakeBucket(ctx, cfg.Bucket, minio.MakeBucketOptions{})
		if err != nil {
			return nil, errors.Wrap(err, "client.MakeBucket")
		}
	}

	return be, nil
}

// isAccessDenied returns true if the error is caused by Access Denied.
func isAccessDenied(err error) bool {
	debug.Log("isAccessDenied(%T, %#v)", err, err)

	var e minio.ErrorResponse
	return errors.As(err, &e) && e.Code == "AccessDenied"
}

// IsNotExist returns true if the error is caused by a not existing file.
func (be *Backend) IsNotExist(err error) bool {
	var e minio.ErrorResponse
	return errors.As(err, &e) && e.Code == "NoSuchKey"
}

func (be *Backend) IsPermanentError(err error) bool {
	if be.IsNotExist(err) {
		return true
	}

	var merr minio.ErrorResponse
	if errors.As(err, &merr) {
		if merr.Code == "InvalidRange" || merr.Code == "AccessDenied" {
			return true
		}
	}

	return false
}

// Join combines path components with slashes.
func (be *Backend) Join(p ...string) string {
	return path.Join(p...)
}

type fileInfo struct {
	name    string
	size    int64
	mode    os.FileMode
	modTime time.Time
	isDir   bool
}

func (fi *fileInfo) Name() string       { return fi.name }    // base name of the file
func (fi *fileInfo) Size() int64        { return fi.size }    // length in bytes for regular files; system-dependent for others
func (fi *fileInfo) Mode() os.FileMode  { return fi.mode }    // file mode bits
func (fi *fileInfo) ModTime() time.Time { return fi.modTime } // modification time
func (fi *fileInfo) IsDir() bool        { return fi.isDir }   // abbreviation for Mode().IsDir()
func (fi *fileInfo) Sys() interface{}   { return nil }        // underlying data source (can return nil)

// ReadDir returns the entries for a directory.
func (be *Backend) ReadDir(ctx context.Context, dir string) (list []os.FileInfo, err error) {
	debug.Log("ReadDir(%v)", dir)

	// make sure dir ends with a slash
	if dir[len(dir)-1] != '/' {
		dir += "/"
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	debug.Log("using ListObjectsV1(%v)", be.cfg.ListObjectsV1)

	for obj := range be.client.ListObjects(ctx, be.cfg.Bucket, minio.ListObjectsOptions{
		Prefix:    dir,
		Recursive: false,
		UseV1:     be.cfg.ListObjectsV1,
	}) {
		if obj.Err != nil {
			return nil, err
		}

		if obj.Key == "" {
			continue
		}

		name := strings.TrimPrefix(obj.Key, dir)
		// Sometimes s3 returns an entry for the dir itself. Ignore it.
		if name == "" {
			continue
		}
		entry := &fileInfo{
			name:    name,
			size:    obj.Size,
			modTime: obj.LastModified,
		}

		if name[len(name)-1] == '/' {
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

func (be *Backend) Connections() uint {
	return be.cfg.Connections
}

// Hasher may return a hash function for calculating a content hash for the backend
func (be *Backend) Hasher() hash.Hash {
	return nil
}

// HasAtomicReplace returns whether Save() can atomically replace files
func (be *Backend) HasAtomicReplace() bool {
	return true
}

// Path returns the path in the bucket that is used for this backend.
func (be *Backend) Path() string {
	return be.cfg.Prefix
}

// useStorageClass returns whether file should be saved in the provided Storage Class
// For archive storage classes, only data files are stored using that class; metadata
// must remain instantly accessible.
func (be *Backend) useStorageClass(h backend.Handle) bool {
	notArchiveClass := be.cfg.StorageClass != "GLACIER" && be.cfg.StorageClass != "DEEP_ARCHIVE"
	isDataFile := h.Type == backend.PackFile && !h.IsMetadata
	return isDataFile || notArchiveClass
}

// Save stores data in the backend at the handle.
func (be *Backend) Save(ctx context.Context, h backend.Handle, rd backend.RewindReader) error {
	objName := be.Filename(h)

	opts := minio.PutObjectOptions{
		ContentType: "application/octet-stream",
		// the only option with the high-level api is to let the library handle the checksum computation
		SendContentMd5: true,
		// only use multipart uploads for very large files
		PartSize: 200 * 1024 * 1024,
	}
	if be.useStorageClass(h) {
		opts.StorageClass = be.cfg.StorageClass
	}

	info, err := be.client.PutObject(ctx, be.cfg.Bucket, objName, io.NopCloser(rd), int64(rd.Length()), opts)

	// sanity check
	if err == nil && info.Size != rd.Length() {
		return errors.Errorf("wrote %d bytes instead of the expected %d bytes", info.Size, rd.Length())
	}

	return errors.Wrap(err, "client.PutObject")
}

// Load runs fn with a reader that yields the contents of the file at h at the
// given offset.
func (be *Backend) Load(ctx context.Context, h backend.Handle, length int, offset int64, fn func(rd io.Reader) error) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	return util.DefaultLoad(ctx, h, length, offset, be.openReader, fn)
}

func (be *Backend) openReader(ctx context.Context, h backend.Handle, length int, offset int64) (io.ReadCloser, error) {
	objName := be.Filename(h)
	opts := minio.GetObjectOptions{}

	var err error
	if length > 0 {
		err = opts.SetRange(offset, offset+int64(length)-1)
	} else if offset > 0 {
		err = opts.SetRange(offset, 0)
	}

	if err != nil {
		return nil, errors.Wrap(err, "SetRange")
	}

	coreClient := minio.Core{Client: be.client}
	rd, info, _, err := coreClient.GetObject(ctx, be.cfg.Bucket, objName, opts)
	if err != nil {
		return nil, err
	}

	if feature.Flag.Enabled(feature.BackendErrorRedesign) && length > 0 {
		if info.Size > 0 && info.Size != int64(length) {
			_ = rd.Close()
			return nil, minio.ErrorResponse{Code: "InvalidRange", Message: "restic-file-too-short"}
		}
	}

	return rd, err
}

// Stat returns information about a blob.
func (be *Backend) Stat(ctx context.Context, h backend.Handle) (bi backend.FileInfo, err error) {
	objName := be.Filename(h)
	var obj *minio.Object

	opts := minio.GetObjectOptions{}

	obj, err = be.client.GetObject(ctx, be.cfg.Bucket, objName, opts)
	if err != nil {
		return backend.FileInfo{}, errors.Wrap(err, "client.GetObject")
	}

	// make sure that the object is closed properly.
	defer func() {
		e := obj.Close()
		if err == nil {
			err = errors.Wrap(e, "Close")
		}
	}()

	fi, err := obj.Stat()
	if err != nil {
		return backend.FileInfo{}, errors.Wrap(err, "Stat")
	}

	return backend.FileInfo{Size: fi.Size, Name: h.Name}, nil
}

// Remove removes the blob with the given name and type.
func (be *Backend) Remove(ctx context.Context, h backend.Handle) error {
	objName := be.Filename(h)

	err := be.client.RemoveObject(ctx, be.cfg.Bucket, objName, minio.RemoveObjectOptions{})

	if be.IsNotExist(err) {
		err = nil
	}

	return errors.Wrap(err, "client.RemoveObject")
}

// List runs fn for each file in the backend which has the type t. When an
// error occurs (or fn returns an error), List stops and returns it.
func (be *Backend) List(ctx context.Context, t backend.FileType, fn func(backend.FileInfo) error) error {
	prefix, recursive := be.Basedir(t)

	// make sure prefix ends with a slash
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	debug.Log("using ListObjectsV1(%v)", be.cfg.ListObjectsV1)

	// NB: unfortunately we can't protect this with be.sem.GetToken() here.
	// Doing so would enable a deadlock situation (gh-1399), as ListObjects()
	// starts its own goroutine and returns results via a channel.
	listresp := be.client.ListObjects(ctx, be.cfg.Bucket, minio.ListObjectsOptions{
		Prefix:    prefix,
		Recursive: recursive,
		UseV1:     be.cfg.ListObjectsV1,
	})

	for obj := range listresp {
		if obj.Err != nil {
			return obj.Err
		}

		m := strings.TrimPrefix(obj.Key, prefix)
		if m == "" {
			continue
		}

		fi := backend.FileInfo{
			Name: path.Base(m),
			Size: obj.Size,
		}

		if ctx.Err() != nil {
			return ctx.Err()
		}

		err := fn(fi)
		if err != nil {
			return err
		}

		if ctx.Err() != nil {
			return ctx.Err()
		}
	}

	return ctx.Err()
}

// Delete removes all restic keys in the bucket. It will not remove the bucket itself.
func (be *Backend) Delete(ctx context.Context) error {
	return util.DefaultDelete(ctx, be)
}

// Close does nothing
func (be *Backend) Close() error { return nil }

// Rename moves a file based on the new layout l.
func (be *Backend) Rename(ctx context.Context, h backend.Handle, l layout.Layout) error {
	debug.Log("Rename %v to %v", h, l)
	oldname := be.Filename(h)
	newname := l.Filename(h)

	if oldname == newname {
		debug.Log("  %v is already renamed", newname)
		return nil
	}

	debug.Log("  %v -> %v", oldname, newname)

	src := minio.CopySrcOptions{
		Bucket: be.cfg.Bucket,
		Object: oldname,
	}

	dst := minio.CopyDestOptions{
		Bucket: be.cfg.Bucket,
		Object: newname,
	}

	_, err := be.client.CopyObject(ctx, dst, src)
	if err != nil && be.IsNotExist(err) {
		debug.Log("copy failed: %v, seems to already have been renamed", err)
		return nil
	}

	if err != nil {
		debug.Log("copy failed: %v", err)
		return err
	}

	return be.client.RemoveObject(ctx, be.cfg.Bucket, oldname, minio.RemoveObjectOptions{})
}
