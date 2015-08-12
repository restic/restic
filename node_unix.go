//

// +build dragonfly linux netbsd openbsd freebsd solaris darwin

package restic

import (
	"syscall"
)

var mknod = syscall.Mknod

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

func (s statUnix) dev() uint64   { return s.Dev }
func (s statUnix) ino() uint64   { return s.Ino }
func (s statUnix) nlink() uint64 { return s.Nlink }
func (s statUnix) uid() uint32   { return s.Uid }
func (s statUnix) gid() uint32   { return s.Gid }
func (s statUnix) rdev() uint64  { return s.Rdev }
func (s statUnix) size() int64   { return s.Size }
