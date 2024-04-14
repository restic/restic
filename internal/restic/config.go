package restic

import (
	"context"
	"sync"
	"testing"

	"github.com/restic/restic/internal/errors"

	"github.com/restic/restic/internal/debug"

	"github.com/restic/chunker"
)

// Config contains the configuration for a repository.
type Config struct {
	Version           uint        `json:"version"`
	ID                string      `json:"id"`
	ChunkerPolynomial chunker.Pol `json:"chunker_polynomial"`
}

const MinRepoVersion = 1
const MaxRepoVersion = 2

// StableRepoVersion is the version that is written to the config when a repository
// is newly created with Init().
const StableRepoVersion = 2

// JSONUnpackedLoader loads unpacked JSON.
type JSONUnpackedLoader interface {
	LoadJSONUnpacked(context.Context, FileType, ID, interface{}) error
}

// CreateConfig creates a config file with a randomly selected polynomial and
// ID.
func CreateConfig(version uint) (Config, error) {
	var (
		err error
		cfg Config
	)

	cfg.ChunkerPolynomial, err = chunker.RandomPolynomial()
	if err != nil {
		return Config{}, errors.Wrap(err, "chunker.RandomPolynomial")
	}

	cfg.ID = NewRandomID().String()
	cfg.Version = version

	debug.Log("New config: %#v", cfg)
	return cfg, nil
}

var checkPolynomial = true
var checkPolynomialOnce sync.Once

// TestDisableCheckPolynomial disables the check that the polynomial used for
// the chunker.
func TestDisableCheckPolynomial(t testing.TB) {
	t.Logf("disabling check of the chunker polynomial")
	checkPolynomialOnce.Do(func() {
		checkPolynomial = false
	})
}

// LoadConfig returns loads, checks and returns the config for a repository.
func LoadConfig(ctx context.Context, r LoaderUnpacked) (Config, error) {
	var (
		cfg Config
	)

	err := LoadJSONUnpacked(ctx, r, ConfigFile, ID{}, &cfg)
	if err != nil {
		return Config{}, err
	}

	if cfg.Version < MinRepoVersion || cfg.Version > MaxRepoVersion {
		return Config{}, errors.Errorf("unsupported repository version %v", cfg.Version)
	}

	if checkPolynomial {
		if !cfg.ChunkerPolynomial.Irreducible() {
			return Config{}, errors.New("invalid chunker polynomial")
		}
	}

	return cfg, nil
}

func SaveConfig(ctx context.Context, r SaverUnpacked, cfg Config) error {
	_, err := SaveJSONUnpacked(ctx, r, ConfigFile, cfg)
	return err
}
