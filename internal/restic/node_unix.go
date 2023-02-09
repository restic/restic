//go:build !windows
// +build !windows

package restic

import (
	"os"
	"syscall"
)

func lchown(name string, uid, gid int) error {
	return os.Lchown(name, uid, gid)
}

type statT syscall.Stat_t

func toStatT(i interface{}) (*statT, bool) {
	s, ok := i.(*syscall.Stat_t)
	if ok && s != nil {
		return (*statT)(s), true
	}
	return nil, false
}

func (s statT) dev() uint64   { return uint64(s.Dev) }
func (s statT) ino() uint64   { return uint64(s.Ino) }
func (s statT) nlink() uint64 { return uint64(s.Nlink) }
func (s statT) uid() uint32   { return uint32(s.Uid) }
func (s statT) gid() uint32   { return uint32(s.Gid) }
func (s statT) rdev() uint64  { return uint64(s.Rdev) }
func (s statT) size() int64   { return int64(s.Size) }
