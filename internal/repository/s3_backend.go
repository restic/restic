package repository

import (
	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/backend/s3"
)

// AsS3Backend extracts the S3 backend from a repository
// TODO remove me once restic 0.17 was released
func AsS3Backend(repo *Repository) *s3.Backend {
	return backend.AsBackend[*s3.Backend](repo.be)
}
