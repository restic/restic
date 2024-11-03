package fs

import (
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/restic"
	"golang.org/x/sys/windows"
)

func (p *pathMetadataHandle) Xattr(_ bool) ([]restic.ExtendedAttribute, error) {
	return xattrFromPath(p.name)
}

func xattrFromPath(path string) ([]restic.ExtendedAttribute, error) {
	allowExtended, err := checkAndStoreEASupport(path)
	if err != nil || !allowExtended {
		return nil, err
	}

	var fileHandle windows.Handle
	if fileHandle, err = openHandleForEA(path, false); err != nil {
		return nil, errors.Errorf("get EA failed while opening file handle for path %v, with: %v", path, err)
	}
	defer closeFileHandle(fileHandle, path)
	//Get the windows Extended Attributes using the file handle
	var extAtts []extendedAttribute
	extAtts, err = fgetEA(fileHandle)
	debug.Log("fillExtendedAttributes(%v) %v", path, extAtts)
	if err != nil {
		return nil, errors.Errorf("get EA failed for path %v, with: %v", path, err)
	}
	if len(extAtts) == 0 {
		return nil, nil
	}

	extendedAttrs := make([]restic.ExtendedAttribute, 0, len(extAtts))
	for _, attr := range extAtts {
		extendedAttr := restic.ExtendedAttribute{
			Name:  attr.Name,
			Value: attr.Value,
		}

		extendedAttrs = append(extendedAttrs, extendedAttr)
	}
	return extendedAttrs, nil
}

func (p *pathMetadataHandle) SecurityDescriptor() (*[]byte, error) {
	return getSecurityDescriptor(p.name)
}

func (p *fdMetadataHandle) Xattr(_ bool) ([]restic.ExtendedAttribute, error) {
	// FIXME
	return xattrFromPath(p.name)
}

func (p *fdMetadataHandle) SecurityDescriptor() (*[]byte, error) {
	// FIXME
	return getSecurityDescriptor(p.name)
}
