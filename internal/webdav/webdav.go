package webdav

import (
	"context"
	"log"
	"net/http"
	"os"

	"golang.org/x/net/webdav"
)

// WebDAV implements a WebDAV handler on the repo.
type WebDAV struct {
	webdav.Handler
}

var logger = log.New(os.Stderr, "webdav log: ", log.Flags())

func logRequest(req *http.Request, err error) {
	logger.Printf("req %v %v -> %v\n", req.Method, req.URL.Path, err)
}

// NewWebDAV returns a new *WebDAV which allows serving the repo via WebDAV.
func NewWebDAV(ctx context.Context, root fuseDir) (*WebDAV, error) {
	fs := &RepoFileSystem{ctx: ctx, root: root}
	wd := &WebDAV{
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
