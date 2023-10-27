package restic

import (
	"context"
	"fmt"
)

type fileInfo struct {
	id   ID
	size int64
}

type memorizedLister struct {
	fileInfos []fileInfo
	tpe       FileType
}

func (m *memorizedLister) List(ctx context.Context, t FileType, fn func(ID, int64) error) error {
	if t != m.tpe {
		return fmt.Errorf("filetype mismatch, expected %s got %s", m.tpe, t)
	}
	for _, fi := range m.fileInfos {
		if ctx.Err() != nil {
			break
		}
		err := fn(fi.id, fi.size)
		if err != nil {
			return err
		}
	}
	return ctx.Err()
}

func MemorizeList(ctx context.Context, be Lister, t FileType) (Lister, error) {
	if _, ok := be.(*memorizedLister); ok {
		return be, nil
	}

	var fileInfos []fileInfo
	err := be.List(ctx, t, func(id ID, size int64) error {
		fileInfos = append(fileInfos, fileInfo{id, size})
		return nil
	})
	if err != nil {
		return nil, err
	}

	return &memorizedLister{
		fileInfos: fileInfos,
		tpe:       t,
	}, nil
}
