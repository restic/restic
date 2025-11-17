package selfupdate

import (
	"context"
	"net/http"
	"testing"

	rtest "github.com/restic/restic/internal/test"
)

func TestNewGitHubRequest(t *testing.T) {
	ctx := context.Background()
	url := "https://api.github.com/repos/restic/restic/releases/latest"
	acceptHeader := "application/vnd.github.v3+json"

	t.Run("With GITHUB_ACCESS_TOKEN", func(t *testing.T) {
		expectedToken := "testtoken123"
		t.Setenv("GITHUB_ACCESS_TOKEN", expectedToken)

		req, err := newGitHubRequest(ctx, url, acceptHeader)
		rtest.OK(t, err)

		rtest.Assert(t, req.Method == http.MethodGet, "expected method %s, got %s", http.MethodGet, req.Method)
		rtest.Assert(t, req.URL.String() == url, "expected URL %s, got %s", url, req.URL.String())
		rtest.Assert(t, req.Header.Get("Accept") == acceptHeader, "expected Accept header %s, got %s", acceptHeader, req.Header.Get("Accept"))
		rtest.Assert(t, req.Header.Get("Authorization") == "token "+expectedToken, "expected Authorization header 'token %s', got %s", expectedToken, req.Header.Get("Authorization"))
	})

	t.Run("Without GITHUB_ACCESS_TOKEN", func(t *testing.T) {
		t.Setenv("GITHUB_ACCESS_TOKEN", "")

		req, err := newGitHubRequest(ctx, url, acceptHeader)
		rtest.OK(t, err)

		rtest.Assert(t, req.Header.Get("Authorization") == "", "expected no Authorization header, got %s", req.Header.Get("Authorization"))
	})
}
