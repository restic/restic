package rest

import (
	"net/url"
	"os"
	"strings"

	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/options"
)

// Config contains all configuration necessary to connect to a REST server.
type Config struct {
	URL         *url.URL
	Connections uint `option:"connections" help:"set a limit for the number of concurrent connections (default: 5)"`
}

func init() {
	options.Register("rest", Config{})
}

// NewConfig returns a new Config with the default values filled in.
func NewConfig() Config {
	return Config{
		Connections: 5,
	}
}

// ParseConfig parses the string s and extracts the REST server URL.
func ParseConfig(s string) (*Config, error) {
	if !strings.HasPrefix(s, "rest:") {
		return nil, errors.New("invalid REST backend specification")
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
	scheme := s[:5]
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
	s = s[5:]
	if !strings.HasSuffix(s, "/") {
		s += "/"
	}
	return s
}

var _ backend.ApplyEnvironmenter = &Config{}

// ApplyEnvironment saves values from the environment to the config.
func (cfg *Config) ApplyEnvironment(prefix string) {
	username := cfg.URL.User.Username()
	pwd, pwdSet := cfg.URL.User.Password()

	// Fall back to the environment for whichever of username/password is not
	// already set in the URL, instead of requiring both to be absent.
	if username == "" {
		username = os.Getenv(prefix + "RESTIC_REST_USERNAME")
	}
	if !pwdSet {
		pwd = os.Getenv(prefix + "RESTIC_REST_PASSWORD")
	}

	if username != "" || pwd != "" {
		cfg.URL.User = url.UserPassword(username, pwd)
	}
}
