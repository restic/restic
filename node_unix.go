// +build dragonfly linux netbsd openbsd freebsd solaris darwin

package restic

import (
	"os"
	"syscall"
)

var mknod = syscall.Mknod
var lchown = os.Lchown

type statUnix syscall.Stat_t

func toStatT(i interface{}) (statT, bool) {
	if i == nil {
		return nil, false
	}
	s, ok := i.(*syscall.Stat_t)
	if ok && s != nil {
		return statUnix(*s), true
	}
	return nil, false
}

func (s statUnix) dev() uint64   { return uint64(s.Dev) }
func (s statUnix) ino() uint64   { return uint64(s.Ino) }
func (s statUnix) nlink() uint64 { return uint64(s.Nlink) }
func (s statUnix) uid() uint32   { return uint32(s.Uid) }
func (s statUnix) gid() uint32   { return uint32(s.Gid) }
func (s statUnix) rdev() uint64  { return uint64(s.Rdev) }
func (s statUnix) size() int64   { return int64(s.Size) }
