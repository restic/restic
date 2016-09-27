package restic

import (
	"testing"

	"restic/errors"

	"restic/debug"

	"github.com/restic/chunker"
)

// Config contains the configuration for a repository.
type Config struct {
	Version           uint        `json:"version"`
	ID                string      `json:"id"`
	ChunkerPolynomial chunker.Pol `json:"chunker_polynomial"`
}

// RepoVersion is the version that is written to the config when a repository
// is newly created with Init().
const RepoVersion = 1

// JSONUnpackedLoader loads unpacked JSON.
type JSONUnpackedLoader interface {
	LoadJSONUnpacked(FileType, ID, interface{}) error
}

// CreateConfig creates a config file with a randomly selected polynomial and
// ID.
func CreateConfig() (Config, error) {
	var (
		err error
		cfg Config
	)

	cfg.ChunkerPolynomial, err = chunker.RandomPolynomial()
	if err != nil {
		return Config{}, errors.Wrap(err, "chunker.RandomPolynomial")
	}

	cfg.ID = NewRandomID().String()
	cfg.Version = RepoVersion

	debug.Log("New config: %#v", cfg)
	return cfg, nil
}

// TestCreateConfig creates a config for use within tests.
func TestCreateConfig(t testing.TB, pol chunker.Pol) (cfg Config) {
	cfg.ChunkerPolynomial = pol

	cfg.ID = NewRandomID().String()
	cfg.Version = RepoVersion

	return cfg
}

// LoadConfig returns loads, checks and returns the config for a repository.
func LoadConfig(r JSONUnpackedLoader) (Config, error) {
	var (
		cfg Config
	)

	err := r.LoadJSONUnpacked(ConfigFile, ID{}, &cfg)
	if err != nil {
		return Config{}, err
	}

	if cfg.Version != RepoVersion {
		return Config{}, errors.New("unsupported repository version")
	}

	if !cfg.ChunkerPolynomial.Irreducible() {
		return Config{}, errors.New("invalid chunker polynomial")
	}

	return cfg, nil
}
