package s3

import (
	"context"
	"fmt"
	"hash"
	"io"
	"net/http"
	"os"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v4"
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

var archiveClasses = []string{"GLACIER", "DEEP_ARCHIVE"}

type warmupStatus int

const (
	warmupStatusCold warmupStatus = iota
	warmupStatusWarmingUp
	warmupStatusWarm
	warmupStatusLukewarm
)

func NewFactory() location.Factory {
	return location.NewHTTPBackendFactory("s3", ParseConfig, location.NoPassword, Create, Open)
}

func open(cfg Config, rt http.RoundTripper) (*Backend, error) {
	debug.Log("open, config %#v", cfg)

	if cfg.EnableRestore && !feature.Flag.Enabled(feature.S3Restore) {
		return nil, fmt.Errorf("feature flag `s3-restore` is required to use `-o s3.enable-restore=true`")
	}

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
		Layout: layout.NewDefaultLayout(cfg.Prefix, path.Join),
	}

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
		&credentials.IAM{},
	})
	client := &http.Client{Transport: tr}

	c, err := creds.GetWithContext(&credentials.CredContext{Client: client})
	if err != nil {
		return nil, errors.Wrap(err, "creds.Get")
	}

	if c.SignerType == credentials.SignatureAnonymous {
		// Fail if no credentials were found to prevent repeated attempts to (unsuccessfully) retrieve new credentials.
		// The first attempt still has to timeout which slows down restic usage considerably. Thus, migrate towards forcing
		// users to explicitly decide between authenticated and anonymous access.
		return nil, fmt.Errorf("no credentials found. Use `-o s3.unsafe-anonymous-auth=true` for anonymous authentication")
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
func Open(_ context.Context, cfg Config, rt http.RoundTripper) (backend.Backend, error) {
	return open(cfg, rt)
}

// Create opens the S3 backend at bucket and region and creates the bucket if
// it does not exist yet.
func Create(ctx context.Context, cfg Config, rt http.RoundTripper) (backend.Backend, error) {
	be, err := open(cfg, rt)
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

func (be *Backend) Properties() backend.Properties {
	return backend.Properties{
		Connections:      be.cfg.Connections,
		HasAtomicReplace: true,
	}
}

// Hasher may return a hash function for calculating a content hash for the backend
func (be *Backend) Hasher() hash.Hash {
	return nil
}

// Path returns the path in the bucket that is used for this backend.
func (be *Backend) Path() string {
	return be.cfg.Prefix
}

// useStorageClass returns whether file should be saved in the provided Storage Class
// For archive storage classes, only data files are stored using that class; metadata
// must remain instantly accessible.
func (be *Backend) useStorageClass(h backend.Handle) bool {
	isDataFile := h.Type == backend.PackFile && !h.IsMetadata
	isArchiveClass := slices.Contains(archiveClasses, be.cfg.StorageClass)
	return !isArchiveClass || isDataFile
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

	info, err := be.client.PutObject(ctx, be.cfg.Bucket, objName, io.NopCloser(rd), rd.Length(), opts)

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

// Warmup transitions handles from cold to hot storage if needed.
func (be *Backend) Warmup(ctx context.Context, handles []backend.Handle) ([]backend.Handle, error) {
	handlesWarmingUp := []backend.Handle{}

	if be.cfg.EnableRestore {
		for _, h := range handles {
			filename := be.Filename(h)
			isWarmingUp, err := be.requestRestore(ctx, filename)
			if err != nil {
				return handlesWarmingUp, err
			}
			if isWarmingUp {
				debug.Log("s3 file is being restored: %s", filename)
				handlesWarmingUp = append(handlesWarmingUp, h)
			}
		}
	}

	return handlesWarmingUp, nil
}

// requestRestore sends a glacier restore request on a given file.
func (be *Backend) requestRestore(ctx context.Context, filename string) (bool, error) {
	objectInfo, err := be.client.StatObject(ctx, be.cfg.Bucket, filename, minio.StatObjectOptions{})
	if err != nil {
		return false, err
	}

	ws := be.getWarmupStatus(objectInfo)
	switch ws {
	case warmupStatusWarm:
		return false, nil
	case warmupStatusWarmingUp:
		return true, nil
	}

	opts := minio.RestoreRequest{}
	opts.SetDays(be.cfg.RestoreDays)
	opts.SetGlacierJobParameters(minio.GlacierJobParameters{Tier: minio.TierType(be.cfg.RestoreTier)})

	if err := be.client.RestoreObject(ctx, be.cfg.Bucket, filename, "", opts); err != nil {
		var e minio.ErrorResponse
		if errors.As(err, &e) {
			switch e.Code {
			case "InvalidObjectState":
				return false, nil
			case "RestoreAlreadyInProgress":
				return true, nil
			}
		}
		return false, err
	}

	isWarmingUp := ws != warmupStatusLukewarm
	return isWarmingUp, nil
}

// getWarmupStatus returns the warmup status of the provided object.
func (be *Backend) getWarmupStatus(objectInfo minio.ObjectInfo) warmupStatus {
	// We can't use objectInfo.StorageClass to get the storage class of the
	// object because this field is only set during ListObjects operations.
	// The response header is the documented way to get the storage class
	// for GetObject/StatObject operations.
	storageClass := objectInfo.Metadata.Get("X-Amz-Storage-Class")
	isArchiveClass := slices.Contains(archiveClasses, storageClass)
	if !isArchiveClass {
		return warmupStatusWarm
	}

	restore := objectInfo.Restore
	if restore != nil {
		if restore.OngoingRestore {
			return warmupStatusWarmingUp
		}

		minExpiryTime := time.Now().Add(time.Duration(be.cfg.RestoreDays) * 24 * time.Hour)
		expiryTime := restore.ExpiryTime
		if !expiryTime.IsZero() {
			if minExpiryTime.Before(expiryTime) {
				return warmupStatusWarm
			}
			return warmupStatusLukewarm
		}
	}

	return warmupStatusCold
}

// WarmupWait waits until all handles are in hot storage.
func (be *Backend) WarmupWait(ctx context.Context, handles []backend.Handle) error {
	timeoutCtx, timeoutCtxCancel := context.WithTimeout(ctx, be.cfg.RestoreTimeout)
	defer timeoutCtxCancel()

	if be.cfg.EnableRestore {
		for _, h := range handles {
			filename := be.Filename(h)
			err := be.waitForRestore(timeoutCtx, filename)
			if err != nil {
				return err
			}
			debug.Log("s3 file is restored: %s", filename)
		}
	}

	return nil
}

// waitForRestore waits for a given file to be restored.
func (be *Backend) waitForRestore(ctx context.Context, filename string) error {
	for {
		var objectInfo minio.ObjectInfo

		// Restore requests can last many hours, therefore network may fail
		// temporarily. We don't need to die in such even.
		b := backoff.WithMaxRetries(backoff.NewExponentialBackOff(), 10)
		b = backoff.WithContext(b, ctx)
		err := backoff.Retry(
			func() (err error) {
				objectInfo, err = be.client.StatObject(ctx, be.cfg.Bucket, filename, minio.StatObjectOptions{})
				return
			},
			b,
		)
		if err != nil {
			return err
		}

		ws := be.getWarmupStatus(objectInfo)
		switch ws {
		case warmupStatusLukewarm:
			fallthrough
		case warmupStatusWarm:
			return nil
		case warmupStatusCold:
			return errors.New("waiting on S3 handle that is not warming up")
		}

		select {
		case <-time.After(1 * time.Minute):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}
