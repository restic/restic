package local

import (
	"strings"

	"restic/errors"
)

// ParseConfig parses a local backend config.
func ParseConfig(cfg string) (interface{}, error) {
	if !strings.HasPrefix(cfg, "local:") {
		return nil, errors.New(`invalid format, prefix "local" not found`)
	}

	return cfg[6:], nil
}
