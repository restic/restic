package fs

import (
	"os"

	"github.com/restic/restic/internal/restic"
)

type metadataHandle interface {
	Name() string
	Stat() (*ExtendedFileInfo, error)
	Readlink() (string, error)
	Xattr(ignoreListError bool) ([]restic.ExtendedAttribute, error)
	// windows only
	SecurityDescriptor() (*[]byte, error)
}

type pathMetadataHandle struct {
	name string
	flag int
}

var _ metadataHandle = &pathMetadataHandle{}

func newPathMetadataHandle(name string, flag int) *pathMetadataHandle {
	return &pathMetadataHandle{
		name: fixpath(name),
		flag: flag,
	}
}

func (p *pathMetadataHandle) Name() string {
	return p.name
}

func (p *pathMetadataHandle) Stat() (*ExtendedFileInfo, error) {
	var fi os.FileInfo
	var err error
	if p.flag&O_NOFOLLOW != 0 {
		fi, err = os.Lstat(p.name)
	} else {
		fi, err = os.Stat(p.name)
	}
	if err != nil {
		return nil, err
	}
	return extendedStat(fi), nil
}

func (p *pathMetadataHandle) Readlink() (string, error) {
	return os.Readlink(p.name)
}
