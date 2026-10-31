package drime

import (
	"os"
	"path"
	"strings"

	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/options"
)

// Config contains all configuration necessary to connect to Drime.
type Config struct {
	AccessToken options.SecretString
	Path        string

	WorkspaceID  string `option:"workspace-id" help:"use this Drime workspace instead of the personal workspace (default: 0)"`
	RootFolderID string `option:"root-folder-id" help:"ID of the Drime folder to use as the root, the repository path is relative to it (default: root of the drive)"`
	Connections  uint   `option:"connections" help:"set a limit for the number of concurrent connections (default: 5)"`

	// Endpoint overrides the API base URL, only used for testing.
	Endpoint string
}

// NewConfig returns a new config with default options applied.
func NewConfig() Config {
	return Config{
		WorkspaceID: "0",
		Connections: 5,
		Endpoint:    defaultEndpoint,
	}
}

func init() {
	options.Register("drime", Config{})
}

// ParseConfig parses the string s and extracts the Drime config. The
// supported configuration format is drime:path/to/repo.
func ParseConfig(s string) (*Config, error) {
	data, ok := strings.CutPrefix(s, "drime:")
	if !ok {
		return nil, errors.New("invalid format, want: drime:path/to/repo")
	}

	p := strings.Trim(path.Clean("/"+data), "/")
	if p == "" {
		return nil, errors.New("invalid format, a repository path is required, want: drime:path/to/repo")
	}

	cfg := NewConfig()
	cfg.Path = p
	return &cfg, nil
}

var _ backend.ApplyEnvironmenter = &Config{}

// ApplyEnvironment saves values from the environment to the config.
func (cfg *Config) ApplyEnvironment(prefix string) {
	if cfg.AccessToken.String() == "" {
		cfg.AccessToken = options.NewSecretString(os.Getenv(prefix + "DRIME_ACCESS_TOKEN"))
	}
	if v := os.Getenv(prefix + "DRIME_WORKSPACE_ID"); v != "" && cfg.WorkspaceID == "0" {
		cfg.WorkspaceID = v
	}
	if cfg.RootFolderID == "" {
		cfg.RootFolderID = os.Getenv(prefix + "DRIME_ROOT_FOLDER_ID")
	}
}
