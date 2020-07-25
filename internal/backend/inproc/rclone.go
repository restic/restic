// +build rclone

package inproc

import (
	"net/http"
	"net/http/httptest"
	"strings"

	_ "github.com/rclone/rclone/backend/all"
	"github.com/rclone/rclone/cmd"
	"github.com/rclone/rclone/cmd/serve/httplib/httpflags"
	"github.com/rclone/rclone/cmd/serve/restic"
)

type rcloneProvider struct {
}

type server struct {
	*restic.Server
}

func (rp *rcloneProvider) NewService(args string) (http.RoundTripper, error) {
	f := cmd.NewFsSrc(strings.Fields(args))
	return &server{restic.NewServer(f, &httpflags.Opt)}, nil
}

func (s *server) RoundTrip(r *http.Request) (*http.Response, error) {
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	return w.Result(), nil
}

func init() {
	RegisterServiceProvider("rclone", &rcloneProvider{})
}
