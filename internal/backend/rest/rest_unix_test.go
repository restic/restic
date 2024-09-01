//go:build !windows
// +build !windows

package rest_test

import (
	"context"
	"fmt"
	"path"
	"testing"

	rtest "github.com/restic/restic/internal/test"
)

func TestBackendRESTWithUnixSocket(t *testing.T) {
	defer func() {
		if t.Skipped() {
			rtest.SkipDisallowed(t, "restic/backend/rest.TestBackendREST")
		}
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dir := rtest.TempDir(t)
	serverURL, cleanup := runRESTServer(ctx, t, path.Join(dir, "data"), fmt.Sprintf("unix:%s", path.Join(dir, "sock")))
	defer cleanup()

	newTestSuite(serverURL, false).RunTests(t)
}
