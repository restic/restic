package repository

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"

	"restic/backend"
	"restic/debug"

	"github.com/restic/chunker"
)

// Config contains the configuration for a repository.
type Config struct {
	Version           uint        `json:"version"`
	ID                string      `json:"id"`
	ChunkerPolynomial chunker.Pol `json:"chunker_polynomial"`
}

// repositoryIDSize is the length of the ID chosen at random for a new repository.
const repositoryIDSize = sha256.Size

// RepoVersion is the version that is written to the config when a repository
// is newly created with Init().
const RepoVersion = 1

// JSONUnpackedSaver saves unpacked JSON.
type JSONUnpackedSaver interface {
	SaveJSONUnpacked(backend.Type, interface{}) (backend.ID, error)
}

// JSONUnpackedLoader loads unpacked JSON.
type JSONUnpackedLoader interface {
	LoadJSONUnpacked(backend.Type, backend.ID, interface{}) error
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
		return Config{}, err
	}

	newID := make([]byte, repositoryIDSize)
	_, err = io.ReadFull(rand.Reader, newID)
	if err != nil {
		return Config{}, err
	}

	cfg.ID = hex.EncodeToString(newID)
	cfg.Version = RepoVersion

	debug.Log("Repo.CreateConfig", "New config: %#v", cfg)
	return cfg, nil
}

// LoadConfig returns loads, checks and returns the config for a repository.
func LoadConfig(r JSONUnpackedLoader) (Config, error) {
	var (
		cfg Config
	)

	err := r.LoadJSONUnpacked(backend.Config, backend.ID{}, &cfg)
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
