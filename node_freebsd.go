package restic

import (
	"os"
	"syscall"
	"time"
)

func (node *Node) OpenForReading() (*os.File, error) {
	file, err := os.OpenFile(node.path, os.O_RDONLY, 0)
	if os.IsPermission(err) {
		return os.OpenFile(node.path, os.O_RDONLY, 0)
	}
	return file, err
}

func (node *Node) fillTimes(stat *syscall.Stat_t) {
	node.ChangeTime = time.Unix(stat.Ctimespec.Unix())
	node.AccessTime = time.Unix(stat.Atimespec.Unix())
}

func changeTime(stat *syscall.Stat_t) time.Time {
	return time.Unix(stat.Ctimespec.Unix())
}
