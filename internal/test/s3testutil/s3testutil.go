// Package s3testutil provides shared helpers for starting a local minio
// server and seeding it with test data. It is intended for use in test files
// across multiple packages and must not be imported by production code.
package s3testutil

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	rtest "github.com/restic/restic/internal/test"
)

// MinioServer holds the connection information for a running local minio test
// server, as returned by StartMinio.
type MinioServer struct {
	Addr   string        // host:port the server listens on (plain HTTP)
	Key    string        // access key
	Secret string        // secret key
	Client *minio.Client // client connected to the server
	Bucket string        // bucket created by StartMinio, empty if none
}

func StartMinio(ctx context.Context, t testing.TB, bucketPrefix string, objects map[string][]byte) *MinioServer {
	t.Helper()

	addr := freeAddr(t)
	key, secret := newCredentials(t)
	cleanup := runMinio(ctx, t, rtest.TempDir(t), key, secret, addr)
	t.Cleanup(cleanup)

	client, err := minio.New(addr, &minio.Options{
		Creds:  credentials.NewStaticV4(key, secret, ""),
		Secure: false,
	})
	if err != nil {
		t.Fatal(err)
	}

	srv := &MinioServer{
		Addr:   addr,
		Key:    key,
		Secret: secret,
		Client: client,
	}

	if bucketPrefix != "" {
		srv.Bucket = fmt.Sprintf("%s-%d", bucketPrefix, time.Now().UnixNano())
		if err := srv.Client.MakeBucket(ctx, srv.Bucket, minio.MakeBucketOptions{}); err != nil {
			t.Fatalf("create bucket %q: %v", srv.Bucket, err)
		}
		UploadObjects(t, ctx, srv.Client, srv.Bucket, objects)
	}

	return srv
}

func (s *MinioServer) SetAWSEnv(t *testing.T) {
	t.Helper()
	t.Setenv("AWS_ENDPOINT_URL", "http://"+s.Addr)
	t.Setenv("AWS_ACCESS_KEY_ID", s.Key)
	t.Setenv("AWS_SECRET_ACCESS_KEY", s.Secret)
}

func SkipIfNotFoundMinio(t testing.TB) bool {
	t.Helper()
	if _, err := exec.LookPath("minio"); err != nil {
		t.Skip(err)
		return true
	}
	return false
}

// freeAddr returns a free TCP address on 127.0.0.1 that can be passed to
// runMinio. The port is released before returning, so there is a small race
// window; this is acceptable for tests.
func freeAddr(t testing.TB) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	return addr
}

// newCredentials returns a random access key and secret key suitable for
// minio. Each value is a 20-character lowercase hex string.
func newCredentials(t testing.TB) (key, secret string) {
	t.Helper()
	buf := make([]byte, 10)

	if _, err := io.ReadFull(rand.Reader, buf); err != nil {
		t.Fatal(err)
	}
	key = hex.EncodeToString(buf)

	if _, err := io.ReadFull(rand.Reader, buf); err != nil {
		t.Fatal(err)
	}
	secret = hex.EncodeToString(buf)
	return key, secret
}

// runMinio starts a minio server listening on addr with the given credentials.
// It creates config and root sub-directories under dir and blocks until the
// server is reachable (up to 20 s). The returned function must be called to
// stop the server.
func runMinio(ctx context.Context, t testing.TB, dir, key, secret, addr string) func() {
	t.Helper()

	for _, sub := range []string{"config", "root"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0700); err != nil {
			t.Fatal(err)
		}
	}

	cmd := exec.CommandContext(ctx, "minio",
		"server",
		"--address", addr,
		"--config-dir", filepath.Join(dir, "config"),
		filepath.Join(dir, "root"),
	)
	cmd.Env = append(os.Environ(),
		"MINIO_ACCESS_KEY="+key,
		"MINIO_SECRET_KEY="+secret,
	)
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	var ok bool
	for i := 0; i < 100; i++ {
		time.Sleep(200 * time.Millisecond)
		c, err := net.Dial("tcp", addr)
		if err == nil {
			ok = true
			_ = c.Close()
			break
		}
	}
	if !ok {
		t.Fatal("unable to connect to minio server")
	}

	return func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}
}

// UploadObjects uploads each entry in objects to bucket. The map key is the
// S3 object key (relative path); the value is the object content.
func UploadObjects(t testing.TB, ctx context.Context, client *minio.Client, bucket string, objects map[string][]byte) {
	t.Helper()
	for key, content := range objects {
		_, err := client.PutObject(ctx, bucket, key,
			bytes.NewReader(content), int64(len(content)),
			minio.PutObjectOptions{})
		if err != nil {
			t.Fatalf("upload %q: %v", key, err)
		}
	}
}
