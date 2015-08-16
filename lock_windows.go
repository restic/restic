package restic

import (
	"os/user"
)

// uidGidInt always returns 0 on Windows, since uid isn't numbers
func uidGidInt(u user.User) (uid, gid uint32, err error) {
	return 0, 0, nil
}
