package s3_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"slices"
	"testing"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/backend/s3"
	"github.com/restic/restic/internal/fs"
	"github.com/restic/restic/internal/options"
	rtest "github.com/restic/restic/internal/test"
)

func TestSourceSeaweedFS(t *testing.T) {
	_, err := exec.LookPath("weed")
	if err != nil {
		t.Skip(err)
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	key, secret := newRandomCredentials(t)
	cleanup := runSeaweedFS(ctx, t, rtest.TempDir(t), key, secret)
	defer cleanup()

	client, err := minio.New(seaweedFSS3Addr, &minio.Options{
		Creds:        credentials.NewStaticV4(key, secret, ""),
		Region:       "us-east-1",
		BucketLookup: minio.BucketLookupPath,
	})
	rtest.OK(t, err)
	const bucket = "sourcetest"
	for i := 0; ; i++ {
		err = client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{})
		if err == nil || i == 50 {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	rtest.OK(t, err)
	put := func(key, content string) {
		_, err := client.PutObject(ctx, bucket, key, bytes.NewReader([]byte(content)), int64(len(content)), minio.PutObjectOptions{})
		rtest.OK(t, err)
	}
	put("dir/a", "version 1")
	put("b¤", "b")

	cfg := s3.NewConfig()
	cfg.Endpoint = seaweedFSS3Addr
	cfg.Bucket = bucket
	cfg.UseHTTP = true
	cfg.Region = "us-east-1"
	cfg.BucketLookup = "path"
	cfg.KeyID = key
	cfg.Secret = options.NewSecretString(secret)
	rt, err := backend.Transport(backend.TransportOptions{})
	rtest.OK(t, err)
	src, err := s3.NewSource(cfg, rt)
	rtest.OK(t, err)

	var keys []string
	rtest.OK(t, src.List(ctx, "", func(obj fs.S3Object) error {
		keys = append(keys, obj.Key)
		return nil
	}))
	slices.Sort(keys)
	rtest.Equals(t, []string{"b¤", "dir/"}, keys)

	obj, err := src.Stat(ctx, "dir/a")
	rtest.OK(t, err)
	rtest.Equals(t, int64(len("version 1")), obj.Size)
	_, err = src.Stat(ctx, "missing")
	rtest.Assert(t, errors.Is(err, os.ErrNotExist), "unexpected error %v", err)

	get := func(key string) (string, fs.S3Object) {
		rd, obj, err := src.Get(ctx, key)
		rtest.OK(t, err)
		buf, err := io.ReadAll(rd)
		rtest.OK(t, err)
		rtest.OK(t, rd.Close())
		return string(buf), obj
	}
	content, getObj := get("dir/a")
	rtest.Equals(t, "version 1", content)
	rtest.Equals(t, obj.ETag, getObj.ETag)
	rtest.Equals(t, obj.Size, getObj.Size)
	rtest.Equals(t, obj.LastModified.Unix(), getObj.LastModified.Unix())

	// the metadata describes the version which is returned
	put("dir/a", "version two")
	content, getObj = get("dir/a")
	rtest.Equals(t, "version two", content)
	rtest.Equals(t, int64(len("version two")), getObj.Size)
	rtest.Assert(t, getObj.ETag != obj.ETag, "ETag did not change")

	_, _, err = src.Get(ctx, "missing")
	rtest.Assert(t, errors.Is(err, os.ErrNotExist), "unexpected error %v", err)
}
