package restic

import (
	"os/user"
)

// UidGidInt always returns 0 on Windows, since uid isn't numbers
func UidGidInt(_ *user.User) (uid, gid uint32, err error) {
	return 0, 0, nil
}
