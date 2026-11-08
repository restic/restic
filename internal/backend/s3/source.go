package s3

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/minio/minio-go/v7"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/fs"
	"github.com/restic/restic/internal/options"
)

// SourceOptions are the extended options for reading a bucket as backup
// source.
type SourceOptions struct {
	Prefetch int `option:"prefetch" help:"number of subdirectories per directory to list in the background with --from-s3, 0 disables it (default: 16)"`
}

func init() {
	options.Register("s3source", SourceOptions{})
}

// ParseSourceOptions returns the s3source.* extended options.
func ParseSourceOptions(o options.Options) (SourceOptions, error) {
	opts := SourceOptions{Prefetch: 16}
	if err := o.Extract("s3source").Apply("s3source", &opts); err != nil {
		return SourceOptions{}, err
	}
	if opts.Prefetch < 0 {
		return SourceOptions{}, errors.Fatalf("s3source.prefetch must not be negative")
	}
	return opts, nil
}

// source provides read access to a bucket for use as a backup source.
type source struct {
	client *minio.Client
	bucket string
	useV1  bool
}

// statically ensure that source implements fs.S3Client.
var _ fs.S3Client = &source{}

// NewSource returns a client for reading the objects in the bucket configured
// in cfg. cfg.Prefix is ignored.
func NewSource(cfg Config, rt http.RoundTripper) (fs.S3Client, error) {
	be, err := open(cfg, rt)
	if err != nil {
		return nil, err
	}
	return &source{
		client: be.client,
		bucket: cfg.Bucket,
		useV1:  cfg.ListObjectsV1,
	}, nil
}

func (s *source) List(ctx context.Context, prefix string, fn func(fs.S3Object) error) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	listresp := s.client.ListObjects(ctx, s.bucket, minio.ListObjectsOptions{
		Prefix:    prefix,
		Recursive: false,
		UseV1:     s.useV1,
	})
	for obj := range listresp {
		if obj.Err != nil {
			return obj.Err
		}
		err := fn(fs.S3Object{
			Key:          obj.Key,
			Size:         obj.Size,
			LastModified: obj.LastModified,
			ETag:         obj.ETag,
		})
		if err != nil {
			return err
		}
	}
	// the channel is also closed if the context is canceled, don't return
	// an incomplete listing in that case
	return ctx.Err()
}

func (s *source) Stat(ctx context.Context, key string) (fs.S3Object, error) {
	info, err := s.client.StatObject(ctx, s.bucket, key, minio.StatObjectOptions{})
	if err != nil {
		var e minio.ErrorResponse
		if errors.As(err, &e) && (e.Code == "NoSuchKey" || e.StatusCode == http.StatusNotFound) {
			return fs.S3Object{}, fmt.Errorf("%w: %v", os.ErrNotExist, err)
		}
		return fs.S3Object{}, err
	}
	return fs.S3Object{
		Key:          info.Key,
		Size:         info.Size,
		LastModified: info.LastModified,
		ETag:         info.ETag,
	}, nil
}

func (s *source) Get(ctx context.Context, key string) (io.ReadCloser, fs.S3Object, error) {
	coreClient := minio.Core{Client: s.client}
	rd, info, _, err := coreClient.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		var e minio.ErrorResponse
		if errors.As(err, &e) && (e.Code == "NoSuchKey" || e.StatusCode == http.StatusNotFound) {
			return nil, fs.S3Object{}, fmt.Errorf("%w: %v", os.ErrNotExist, err)
		}
		return nil, fs.S3Object{}, err
	}
	return rd, fs.S3Object{
		Key:          key,
		Size:         info.Size,
		LastModified: info.LastModified,
		ETag:         info.ETag,
	}, nil
}
