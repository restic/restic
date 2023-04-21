package rclone

import (
	"strings"
	"time"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/options"
)

// Config contains all configuration necessary to start rclone.
type Config struct {
	Program     string `option:"program" help:"path to rclone (default: rclone)"`
	Args        string `option:"args"    help:"arguments for running rclone (default: serve restic --stdio --b2-hard-delete)"`
	Remote      string
	Connections uint          `option:"connections" help:"set a limit for the number of concurrent connections (default: 5)"`
	Timeout     time.Duration `option:"timeout"     help:"set a timeout limit to wait for rclone to establish a connection (default: 1m)"`
}

var defaultConfig = Config{
	Program:     "rclone",
	Args:        "serve restic --stdio --b2-hard-delete",
	Connections: 5,
	Timeout:     time.Minute,
}

func init() {
	options.Register("rclone", Config{})
}

// NewConfig returns a new Config with the default values filled in.
func NewConfig() Config {
	return defaultConfig
}

// ParseConfig parses the string s and extracts the remote server URL.
func ParseConfig(s string) (*Config, error) {
	if !strings.HasPrefix(s, "rclone:") {
		return nil, errors.New("invalid rclone backend specification")
	}

	s = s[7:]
	cfg := NewConfig()
	cfg.Remote = s
	return &cfg, nil
}
