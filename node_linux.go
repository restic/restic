package restic

import (
	"os"
	"syscall"
	"time"
)

func (node *Node) OpenForReading() (*os.File, error) {
	file, err := os.OpenFile(node.path, os.O_RDONLY|syscall.O_NOATIME, 0)
	if os.IsPermission(err) {
		return os.OpenFile(node.path, os.O_RDONLY, 0)
	}
	return file, err
}

func (node *Node) fillTimes(stat *syscall.Stat_t) {
	node.ChangeTime = time.Unix(stat.Ctim.Unix())
	node.AccessTime = time.Unix(stat.Atim.Unix())
}

func (node *Node) fillDevice(stat *syscall.Stat_t) {
	node.Device = stat.Rdev
}

func (node *Node) createDevAt(path string) error {
	return syscall.Mknod(path, syscall.S_IFBLK|0600, int(node.Device))
}

func (node *Node) createCharDevAt(path string) error {
	return syscall.Mknod(path, syscall.S_IFCHR|0600, int(node.Device))
}

func (node *Node) createFifoAt(path string) error {
	return syscall.Mkfifo(path, 0600)
}

func changeTime(stat *syscall.Stat_t) time.Time {
	return time.Unix(stat.Ctim.Unix())
}
