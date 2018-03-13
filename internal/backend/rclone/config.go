package rclone

import (
	"strings"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/options"
)

// Config contains all configuration necessary to start rclone.
type Config struct {
	Program string `option:"program" help:"path to rclone (default: rclone)"`
	Args    string `option:"args"    help:"arguments for running rclone (default: restic serve --stdio)"`
	Remote  string
}

func init() {
	options.Register("rclone", Config{})
}

// NewConfig returns a new Config with the default values filled in.
func NewConfig() Config {
	return Config{}
}

// ParseConfig parses the string s and extracts the remote server URL.
func ParseConfig(s string) (interface{}, error) {
	if !strings.HasPrefix(s, "rclone:") {
		return nil, errors.New("invalid rclone backend specification")
	}

	s = s[7:]
	cfg := NewConfig()
	cfg.Remote = s
	return cfg, nil
}
