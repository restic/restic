package rclone

import (
	"strings"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/options"
)

// Config contains all configuration necessary to start rclone.
type Config struct {
	Program     string `option:"program" help:"path to rclone (default: rclone)"`
	Args        string `option:"args"    help:"arguments for running rclone (default: serve restic --stdio)"`
	Remote      string
	Connections uint `option:"connections" help:"set a limit for the number of concurrent connections (default: 5)"`
}

func init() {
	options.Register("rclone", Config{})
}

// NewConfig returns a new Config with the default values filled in.
func NewConfig() Config {
	return Config{
		Connections: 5,
	}
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
