// +build rclone

package inproc

import (
	"net/http"
	"strings"

	_ "github.com/rclone/rclone/backend/all"
	"github.com/rclone/rclone/cmd/serve/restic"
)

type rcloneProvider struct {
}

func (rp *rcloneProvider) NewService(args string) (http.RoundTripper, error) {
	return restic.NewServer(strings.Fields(args))
}

func init() {
	RegisterServiceProvider("rclone", &rcloneProvider{})
}
