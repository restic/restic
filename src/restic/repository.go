package restic

import "restic/repository"

// Repository stores data in a backend. It provides high-level functions and
// transparently encrypts/decrypts data.
type Repository interface {

	// Backend returns the backend used by the repository
	Backend() Backend

	SetIndex(*repository.MasterIndex)
}
