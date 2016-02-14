package sftp

import (
	"errors"
	"net/url"
	"strings"
)

// Config collects all information required to connect to an sftp server.
type Config struct {
	User, Host, Dir string
}

// ParseConfig extracts all information for the sftp connection from the string s.
func ParseConfig(s string) (interface{}, error) {
	if strings.HasPrefix(s, "sftp://") {
		return parseFormat1(s)
	}

	// otherwise parse in the sftp:user@host:path format, which means we'll get
	// "user@host:path" in s
	return parseFormat2(s)
}

// parseFormat1 parses the first format, starting with a slash, so the user
// either specified "sftp://host/path", so we'll get everything after the first
// colon character
func parseFormat1(s string) (Config, error) {
	url, err := url.Parse(s)
	if err != nil {
		return Config{}, err
	}

	cfg := Config{
		Host: url.Host,
		Dir:  url.Path[1:],
	}
	if url.User != nil {
		cfg.User = url.User.Username()
	}
	return cfg, nil
}

// parseFormat2 parses the second format, sftp:user@host:path
func parseFormat2(s string) (cfg Config, err error) {
	// split user/host and path at the second colon
	data := strings.SplitN(s, ":", 3)
	if len(data) < 3 {
		return Config{}, errors.New("sftp: invalid format, hostname or path not found")
	}

	if data[0] != "sftp" {
		return Config{}, errors.New(`invalid format, does not start with "sftp:"`)
	}

	userhost := data[1]
	cfg.Dir = data[2]

	data = strings.SplitN(userhost, "@", 2)
	if len(data) == 2 {
		cfg.User = data[0]
		cfg.Host = data[1]
	} else {
		cfg.Host = userhost
	}

	return cfg, nil
}
