package serve

import (
	"context"
	"log"
	"net/http"
	"os"

	"github.com/restic/restic/internal/restic"
	"golang.org/x/net/webdav"
)

// WebDAV implements a WebDAV handler on the repo.
type WebDAV struct {
	restic.Repository
	webdav.Handler
}

var logger = log.New(os.Stderr, "webdav log: ", log.Flags())

func logRequest(req *http.Request, err error) {
	logger.Printf("req %v %v -> %v\n", req.Method, req.URL.Path, err)
}

// NewWebDAV returns a new *WebDAV which allows serving the repo via WebDAV.
func NewWebDAV(ctx context.Context, repo restic.Repository, cfg Config) (*WebDAV, error) {
	fs, err := NewRepoFileSystem(ctx, repo, cfg)
	if err != nil {
		return nil, err
	}

	wd := &WebDAV{
		Repository: repo,
		Handler: webdav.Handler{
			FileSystem: fs,
			LockSystem: webdav.NewMemLS(),
			Logger:     logRequest,
		},
	}
	return wd, nil
}

func (srv *WebDAV) ServeHTTP(res http.ResponseWriter, req *http.Request) {
	logger.Printf("handle %v %v\n", req.Method, req.URL.Path)
	srv.Handler.ServeHTTP(res, req)
}
