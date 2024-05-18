package azure

import (
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/options"
)

// Config contains all configuration necessary to connect to an azure compatible
// server.
type Config struct {
	AccountName        string
	AccountSAS         options.SecretString
	AccountKey         options.SecretString
	ForceCliCredential bool
	EndpointSuffix     string
	Container          string
	Prefix             string

	Connections uint `option:"connections" help:"set a limit for the number of concurrent connections (default: 5)"`
}

// NewConfig returns a new Config with the default values filled in.
func NewConfig() Config {
	return Config{
		Connections: 5,
	}
}

func init() {
	options.Register("azure", Config{})
}

// ParseConfig parses the string s and extracts the azure config. The
// configuration format is azure:containerName:/[prefix].
func ParseConfig(s string) (*Config, error) {
	if !strings.HasPrefix(s, "azure:") {
		return nil, errors.New("azure: invalid format")
	}

	// strip prefix "azure:"
	s = s[6:]

	// use the first entry of the path as the bucket name and the
	// remainder as prefix
	container, prefix, colon := strings.Cut(s, ":")
	if !colon {
		return nil, errors.New("azure: invalid format: bucket name or path not found")
	}
	prefix = strings.TrimPrefix(path.Clean(prefix), "/")
	cfg := NewConfig()
	cfg.Container = container
	cfg.Prefix = prefix
	return &cfg, nil
}

var _ backend.ApplyEnvironmenter = &Config{}

// ApplyEnvironment saves values from the environment to the config.
func (cfg *Config) ApplyEnvironment(prefix string) {
	if cfg.AccountName == "" {
		cfg.AccountName = os.Getenv(prefix + "AZURE_ACCOUNT_NAME")
	}

	if cfg.AccountKey.String() == "" {
		cfg.AccountKey = options.NewSecretString(os.Getenv(prefix + "AZURE_ACCOUNT_KEY"))
	}

	if cfg.AccountSAS.String() == "" {
		cfg.AccountSAS = options.NewSecretString(os.Getenv(prefix + "AZURE_ACCOUNT_SAS"))
	}

	var forceCliCred, err = strconv.ParseBool(os.Getenv(prefix + "AZURE_FORCE_CLI_CREDENTIAL"))
	if err == nil {
		cfg.ForceCliCredential = forceCliCred
	}

	if cfg.EndpointSuffix == "" {
		cfg.EndpointSuffix = os.Getenv(prefix + "AZURE_ENDPOINT_SUFFIX")
	}
}
