package fs

import (
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/restic"
	"golang.org/x/sys/windows"
)

func (p *pathMetadataHandle) Xattr(_ bool) ([]restic.ExtendedAttribute, error) {
	allowExtended, err := checkAndStoreEASupport(p.name)
	if err != nil || !allowExtended {
		return nil, err
	}

	var fileHandle windows.Handle
	if fileHandle, err = openHandleForEA(p.name, false); err != nil {
		return nil, errors.Errorf("get EA failed while opening file handle for path %v, with: %v", p.name, err)
	}
	defer closeFileHandle(fileHandle, p.name)

	//Get the windows Extended Attributes using the file handle
	var extAtts []extendedAttribute
	extAtts, err = fgetEA(fileHandle)
	debug.Log("fillExtendedAttributes(%v) %v", p.name, extAtts)
	if err != nil {
		return nil, errors.Errorf("get EA failed for path %v, with: %v", p.name, err)
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
