package restic

import "os/user"

func (sn *Snapshot) fillUserInfo() error {
	usr, err := user.Current()
	if err != nil {
		return err
	}

	sn.Username = usr.Username
	sn.UserID = usr.Uid
	sn.GroupID = usr.Gid

	return nil
}
