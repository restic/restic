// +build !windows

package restic

import (
	"os/user"
	"strconv"
)

// uidGidInt returns uid, gid of the user as a number.
func uidGidInt(u user.User) (uid, gid uint32, err error) {
	var ui, gi int
	ui, err = strconv.ParseInt(u.Uid, 10, 32)
	if err != nil {
		return
	}
	gi, err = strconv.ParseInt(u.Gid, 10, 32)
	if err != nil {
		return
	}
	uid = uint32(ui)
	gid = uint32(gi)
	return
}
