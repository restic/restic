package restic

import (
	"context"
	// "fmt"

	"github.com/restic/restic/internal/errors"
)

var ErrOK = errors.New("ok")

var ErrInvalidData = errors.New("invalid data returned")

var ErrIndexIncomplete = errors.Fatal("index is not complete")

var ErrPacksMissing = errors.Fatal("packs from index missing in repo")

var ErrSizeNotMatching = errors.Fatal("pack size does not match calculated size from index")

// ErrNoRepository is used to report if opening a repository failed due
// to a missing backend storage location or config file
var ErrNoRepository = errors.New("repository does not exist")

// ErrInvalidSourceData is used to report an incomplete backup
var ErrInvalidSourceData = errors.New("at least one source file could not be read")

var ErrNegativePolicyCount = errors.New("negative values not allowed, use 'unlimited' instead")

var ErrFailedToRemoveOneOrMoreSnapshots = errors.New("failed to remove one or more snapshots")

var ErrNoKeyFound = errors.New("wrong password or no key found")

func ComputeExitCode(err error) int {
	var exitCode int
	switch {
	case err == nil:
		exitCode = 0
	case err == ErrInvalidSourceData:
		exitCode = 3
	case errors.Is(err, ErrFailedToRemoveOneOrMoreSnapshots):
		exitCode = 3
	case errors.Is(err, ErrNoRepository):
		exitCode = 10
	case IsAlreadyLocked(err):
		exitCode = 11
	case errors.Is(err, ErrNoKeyFound):
		exitCode = 12
	case errors.Is(err, context.Canceled):
		exitCode = 130
	default:
		exitCode = 1
	}
	return exitCode
}
