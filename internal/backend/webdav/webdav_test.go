package webdav_test

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/restic/restic/internal/backend/test"
	"github.com/restic/restic/internal/backend/webdav"
	rtest "github.com/restic/restic/internal/test"
	gowebdav "golang.org/x/net/webdav"
)

func runWebDAVServer(ctx context.Context, t testing.TB) (*url.URL, func()) {
	ctx, cancel := context.WithCancel(ctx)

	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}

	address := ln.Addr().(*net.TCPAddr).AddrPort().String()
	url, err := url.Parse(fmt.Sprintf("http://%s/restic-test/", address))
	if err != nil {
		t.Fatal(err)
	}

	server := &http.Server{
		Addr: address,
		Handler: &gowebdav.Handler{
			FileSystem: gowebdav.NewMemFS(),
			LockSystem: gowebdav.NewMemLS(),
		},
	}

	go func() {
		server.Serve(ln)
	}()

	go func() {
		downCtx, downCancel := context.WithTimeout(context.Background(), time.Second)
		defer downCancel()

		<-ctx.Done()
		server.Shutdown(downCtx)
	}()

	return url, cancel
}

func newTestSuite(url *url.URL, minimalData bool) *test.Suite[webdav.Config] {
	return &test.Suite[webdav.Config]{
		MinimalData: minimalData,
		Factory:     webdav.NewFactory(),
		NewConfig: func() (*webdav.Config, error) {
			cfg := webdav.NewConfig()
			cfg.URL = url
			return &cfg, nil
		},
	}
}

func TestBackendWebDAV(t *testing.T) {
	defer func() {
		if t.Skipped() {
			rtest.SkipDisallowed(t, "restic/backend/webdav.TestBackendWebDAV")
		}
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	serverURL, cleanup := runWebDAVServer(ctx, t)
	defer cleanup()

	newTestSuite(serverURL, false).RunTests(t)
}

func TestBackendWebDAVExternalServer(t *testing.T) {
	repostr := os.Getenv("RESTIC_TEST_WEBDAV_REPOSITORY")
	if repostr == "" {
		t.Skipf("environment variable %v not set", "RESTIC_TEST_WEBDAV_REPOSITORY")
	}

	cfg, err := webdav.ParseConfig(repostr)
	if err != nil {
		t.Fatal(err)
	}

	newTestSuite(cfg.URL, true).RunTests(t)
}

func BenchmarkBackendWebDAV(t *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	serverURL, cleanup := runWebDAVServer(ctx, t)
	defer cleanup()

	newTestSuite(serverURL, false).RunBenchmarks(t)
}
