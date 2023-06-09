package fs

import (
	"os"

	"golang.org/x/sys/unix"
)

func PreallocateFile(wr *os.File, size int64) error {
	// try contiguous first
	fst := unix.Fstore_t{
		Flags:   unix.F_ALLOCATECONTIG | unix.F_ALLOCATEALL,
		Posmode: unix.F_PEOFPOSMODE,
		Offset:  0,
		Length:  size,
	}
	err := unix.FcntlFstore(wr.Fd(), unix.F_PREALLOCATE, &fst)

	if err == nil {
		return nil
	}

	// just take preallocation in any form, but still ask for everything
	fst.Flags = unix.F_ALLOCATEALL
	err = unix.FcntlFstore(wr.Fd(), unix.F_PREALLOCATE, &fst)

	return err
}
