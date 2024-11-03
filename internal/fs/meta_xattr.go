//go:build darwin || freebsd || linux || solaris
// +build darwin freebsd linux solaris

package fs

import (
	"fmt"
	"os"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/restic"
)

func (p *pathMetadataHandle) Xattr(ignoreListError bool) ([]restic.ExtendedAttribute, error) {
	return xattrFromPath(p.Name(), ignoreListError)
}

func xattrFromPath(path string, ignoreListError bool) ([]restic.ExtendedAttribute, error) {
	xattrs, err := listxattr(path)
	debug.Log("fillExtendedAttributes(%v) %v %v", path, xattrs, err)
	if err != nil {
		if ignoreListError && isListxattrPermissionError(err) {
			return nil, nil
		}
		return nil, err
	}

	extendedAttrs := make([]restic.ExtendedAttribute, 0, len(xattrs))
	for _, attr := range xattrs {
		attrVal, err := getxattr(path, attr)
		if err != nil {
			fmt.Fprintf(os.Stderr, "can not obtain extended attribute %v for %v:\n", attr, path)
			continue
		}
		attr := restic.ExtendedAttribute{
			Name:  attr,
			Value: attrVal,
		}

		extendedAttrs = append(extendedAttrs, attr)
	}

	return extendedAttrs, nil
}
