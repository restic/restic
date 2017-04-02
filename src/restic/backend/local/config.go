package local

import (
	"strings"

	"restic/errors"
)

// Config holds all information needed to open a local repository.
type Config struct {
	Path   string
	Layout string
}

// ParseConfig parses a local backend config.
func ParseConfig(cfg string) (interface{}, error) {
	if !strings.HasPrefix(cfg, "local:") {
		return nil, errors.New(`invalid format, prefix "local" not found`)
	}

	return Config{Path: cfg[6:]}, nil
}
