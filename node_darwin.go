package restic

import (
	"os"
	"syscall"
	"time"
)

func (node *Node) OpenForReading() (*os.File, error) {
	return os.Open(node.path)
}

func changeTime(stat *syscall.Stat_t) time.Time {
	return time.Unix(stat.Ctimespec.Unix())
}

func (node *Node) fillTimes(stat *syscall.Stat_t) {
	node.ChangeTime = time.Unix(stat.Ctimespec.Unix())
	node.AccessTime = time.Unix(stat.Atimespec.Unix())
}

func (node Node) restoreSymlinkTimestamps(path string, utimes [2]syscall.Timespec) error {
	return nil
}
