package s3

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/feature"
	rtest "github.com/restic/restic/internal/test"
)

// getMinioMaxRetries uses reflection to read the private maxRetries field
// of a minio.Client.
func getMinioMaxRetries(t *testing.T, c *minio.Client) int {
	t.Helper()
	rv := reflect.ValueOf(c).Elem()
	fv := rv.FieldByName("maxRetries")
	rtest.Assert(t, fv.IsValid(), "maxRetries field not found on minio.Client")
	return int(fv.Int())
}

// newWatchdogTransport creates an http.RoundTripper with the watchdog enabled
// and a short timeout for testing.
func newWatchdogTransport(t *testing.T, timeout time.Duration) http.RoundTripper {
	t.Helper()
	rt, err := backend.Transport(backend.TransportOptions{
		StuckRequestTimeout: timeout,
	})
	rtest.OK(t, err)
	return rt
}

func newTestS3Config(endpoint string) Config {
	return Config{
		Endpoint:            endpoint,
		Bucket:              "test-bucket",
		UseHTTP:             true,
		UnsafeAnonymousAuth: true,
	}
}

// TestOpenSetsMaxRetriesToOneWithWatchdog verifies that s3.open() sets
// minio.Options.MaxRetries to 1 when the BackendErrorRedesign feature flag
// is enabled (which activates the watchdog).
func TestOpenSetsMaxRetriesToOneWithWatchdog(t *testing.T) {
	restore := feature.TestSetFlag(t, feature.Flag, feature.BackendErrorRedesign, true)
	defer restore()

	rt := newWatchdogTransport(t, 100*time.Millisecond)
	cfg := newTestS3Config("127.0.0.1:9000")

	be, err := open(cfg, rt)
	rtest.OK(t, err)
	rtest.OK(t, be.Close())

	maxRetries := getMinioMaxRetries(t, be.client)
	rtest.Equals(t, 1, maxRetries,
		"MaxRetries must be 1 when watchdog is active to prevent silent stalls")
}

// TestOpenRespectsMaxRetriesWithoutWatchdog verifies that s3.open()
// uses the user-supplied MaxRetries when the watchdog is NOT active.
func TestOpenRespectsMaxRetriesWithoutWatchdog(t *testing.T) {
	restore := feature.TestSetFlag(t, feature.Flag, feature.BackendErrorRedesign, false)
	defer restore()

	rt := newWatchdogTransport(t, 100*time.Millisecond)
	cfg := newTestS3Config("127.0.0.1:9000")
	cfg.MaxRetries = 5

	be, err := open(cfg, rt)
	rtest.OK(t, err)
	rtest.OK(t, be.Close())

	maxRetries := getMinioMaxRetries(t, be.client)
	rtest.Equals(t, 5, maxRetries,
		"MaxRetries must respect user-supplied value when watchdog is inactive")
}

// TestOpenMaxRetriesDefaultWithoutWatchdog verifies that s3.open() uses
// minio-go's default MaxRetry (10) when the watchdog is inactive and no
// explicit retry count is set.
func TestOpenMaxRetriesDefaultWithoutWatchdog(t *testing.T) {
	restore := feature.TestSetFlag(t, feature.Flag, feature.BackendErrorRedesign, false)
	defer restore()

	rt := newWatchdogTransport(t, 100*time.Millisecond)
	cfg := newTestS3Config("127.0.0.1:9000")

	be, err := open(cfg, rt)
	rtest.OK(t, err)
	rtest.OK(t, be.Close())

	maxRetries := getMinioMaxRetries(t, be.client)
	rtest.Equals(t, 10, maxRetries,
		"MaxRetries must default to minio-go's default (10) when watchdog is inactive")
}

// TestOpenOverridesUserMaxRetriesWithWatchdog verifies that even if a user
// sets -o s3.retries=N, the watchdog override takes precedence for safety.
func TestOpenOverridesUserMaxRetriesWithWatchdog(t *testing.T) {
	restore := feature.TestSetFlag(t, feature.Flag, feature.BackendErrorRedesign, true)
	defer restore()

	rt := newWatchdogTransport(t, 100*time.Millisecond)
	cfg := newTestS3Config("127.0.0.1:9000")
	cfg.MaxRetries = 10 // user explicitly sets retries

	be, err := open(cfg, rt)
	rtest.OK(t, err)
	rtest.OK(t, be.Close())

	maxRetries := getMinioMaxRetries(t, be.client)
	rtest.Equals(t, 1, maxRetries,
		"MaxRetries must be forced to 1 when watchdog is active, even if user set retries")
}

// TestS3SaveSurfacesErrorQuicklyWithWatchdog is an integration test that
// verifies the fix: when the S3 backend is configured with the watchdog,
// a stalled connection surfaces an error after ~1 watchdog period (not 10×).
func TestS3SaveSurfacesErrorQuicklyWithWatchdog(t *testing.T) {
	t.Parallel()

	restore := feature.TestSetFlag(t, feature.Flag, feature.BackendErrorRedesign, true)
	defer restore()

	var requestCount int32

	// Stalling server: reads body, then never responds (simulates frozen S3)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requestCount, 1)
		_, _ = io.Copy(io.Discard, r.Body)
		_ = r.Body.Close()
		// Stall: never write a response header, simulating a frozen server
		select {
		case <-time.After(10 * time.Second):
		case <-r.Context().Done():
		}
	}))
	defer srv.Close()

	rt := newWatchdogTransport(t, 100*time.Millisecond)
	cfg := newTestS3Config(srv.Listener.Addr().String())

	be, err := open(cfg, rt)
	rtest.OK(t, err)
	defer func() { _ = be.Close() }()

	ctx := context.Background()
	data := bytes.Repeat([]byte("x"), 4096)
	rd := backend.NewByteReader(data, nil)
	h := backend.Handle{
		Type: backend.PackFile,
		Name: "test-pack",
	}

	start := time.Now()
	saveErr := be.Save(ctx, h, rd)
	elapsed := time.Since(start)

	count := int(atomic.LoadInt32(&requestCount))
	t.Logf("Save completed in %v with %d requests, error: %v", elapsed, count, saveErr)

	// With MaxRetries=1, the watchdog error surfaces after ~1 watchdog period (~100ms),
	// not after 10 periods (~1s+). Without the fix, minio-go would silently retry
	// 10 times (10x watchdog = ~1s+), each creating a new stalled request.
	rtest.Assert(t, elapsed < 2*time.Second,
		fmt.Sprintf("error surfaced in %v (expected < 2s). Without fix, minio-go would retry 10 times.", elapsed))
	rtest.Equals(t, 1, count,
		"FIX VERIFIED: only 1 request sent to server (minio-go did not silently retry)")
	rtest.Assert(t, saveErr != nil,
		"Save should return an error for a stalled server")
}
