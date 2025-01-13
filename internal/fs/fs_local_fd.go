//go:build linux || darwin || windows

package fs

import "os"

type fdLocalFile struct {
	localFile
	flag         int
	metadataOnly bool
}

var _ File = &fdLocalFile{}

func newFdLocalFile(name string, flag int, metadataOnly bool) (*fdLocalFile, error) {
	var f *os.File
	var err error
	if metadataOnly {
		f, err = openMetadataHandle(name, flag)
	} else {
		f, err = openReadHandle(name, flag)
	}
	if err != nil {
		return nil, err
	}
	meta := newFdMetadataHandle(name, f)
	return &fdLocalFile{
		localFile: localFile{
			name: name,
			f:    f,
			meta: meta,
		},
		flag:         flag,
		metadataOnly: metadataOnly,
	}, nil
}

func (f *fdLocalFile) MakeReadable() error {
	if !f.metadataOnly {
		panic("file is already readable")
	}

	newF, err := reopenMetadataHandle(f.f)
	// old handle is no longer usable
	f.f = nil
	if err != nil {
		return err
	}
	f.f = newF
	f.meta = newFdMetadataHandle(f.name, newF)
	f.metadataOnly = false
	return nil
}
