package restic

import (
	"fmt"
	"os"
	"os/user"
	"strconv"
	"syscall"
	"time"

	"github.com/restic/restic/debug"
)

func (node *Node) OpenForReading() (*os.File, error) {
	return os.Open(node.path)
}

func (node *Node) fillTimes(stat *syscall.Stat_t) {
	node.ChangeTime = time.Unix(stat.Ctimespec.Unix())
	node.AccessTime = time.Unix(stat.Atimespec.Unix())
}

func (node *Node) fillDevice(stat *syscall.Stat_t) {
	node.Device = uint64(stat.Rdev)
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

func changeTime(stat *syscall.Stat_t) time.Unix {
	return time.Unix(stat.Ctimespec.Unix())
}
