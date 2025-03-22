package fs

import (
	"github.com/restic/restic/internal/errors"
)

func (f *localFile) GetBlockDeviceSize() (uint64, error) {
	return 0, errors.New("Backup of block devices is not supported on Windows")
}
