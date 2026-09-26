package webdav

import (
	"net/url"
	"os"
	"strings"

	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/options"
)

// Config contains all configuration necessary to connect to a WebDAV server.
type Config struct {
	URL         *url.URL
	Connections uint `option:"connections" help:"set a limit for the number of concurrent connections (default: 5)"`
}

func init() {
	options.Register("webdav", Config{})
}

// NewConfig returns a new Config with the default values filled in.
func NewConfig() Config {
	return Config{
		Connections: 5,
	}
}

// ParseConfig parses the string s and extracts the WebDAV server URL.
func ParseConfig(s string) (*Config, error) {
	if !strings.HasPrefix(s, "webdav:") {
		return nil, errors.New("invalid WebDAV backend specification")
	}

	s = prepareURL(s)

	u, err := url.Parse(s)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	cfg := NewConfig()
	cfg.URL = u
	return &cfg, nil
}

// StripPassword removes the password from the URL
// If the repository location cannot be parsed as a valid URL, it will be returned as is
// (it's because this function is used for logging errors)
func StripPassword(s string) string {
	scheme := s[:7]
	s = prepareURL(s)

	u, err := url.Parse(s)
	if err != nil {
		return scheme + s
	}

	if _, set := u.User.Password(); !set {
		return scheme + s
	}

	// a password was set: we replace it with ***
	return scheme + strings.Replace(u.String(), u.User.String()+"@", u.User.Username()+":***@", 1)
}

func prepareURL(s string) string {
	s = s[7:]
	if !strings.HasSuffix(s, "/") {
		s += "/"
	}
	return s
}

var _ backend.ApplyEnvironmenter = &Config{}

// ApplyEnvironment saves values from the environment to the config.
func (cfg *Config) ApplyEnvironment(prefix string) {
	username := cfg.URL.User.Username()
	_, pwdSet := cfg.URL.User.Password()

	// Only apply env variable values if neither username nor password are provided.
	if username == "" && !pwdSet {
		envName := os.Getenv(prefix + "RESTIC_WEBDAV_USERNAME")
		envPwd := os.Getenv(prefix + "RESTIC_WEBDAV_PASSWORD")

		cfg.URL.User = url.UserPassword(envName, envPwd)
	}
}
