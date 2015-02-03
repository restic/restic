package restic

import (
	"os/user"
	"strconv"
)

func (sn *Snapshot) fillUserInfo() error {
	usr, err := user.Current()
	if err != nil {
		return err
	}

	sn.Username = usr.Username
	uid, err := strconv.ParseInt(usr.Uid, 10, 32)
	if err != nil {
		return err
	}
	sn.UID = uint32(uid)

	gid, err := strconv.ParseInt(usr.Gid, 10, 32)
	if err != nil {
		return err
	}
	sn.GID = uint32(gid)

	return nil
}
