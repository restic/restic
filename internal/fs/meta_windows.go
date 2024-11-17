package fs

import (
	"os"
	"syscall"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/restic"
	"golang.org/x/sys/windows"
)

func (p *pathMetadataHandle) Xattr(_ bool) ([]restic.ExtendedAttribute, error) {
	f, err := openMetadataHandle(p.name, p.flag)
	if err != nil {
		return nil, err
	}
	defer func() {
		cerr := f.Close()
		if err == nil {
			err = cerr
		}
	}()

	return xattrFromHandle(p.name, windows.Handle(f.Fd()))
}

func xattrFromHandle(path string, handle windows.Handle) ([]restic.ExtendedAttribute, error) {
	if supp, err := handleSupportsExtendedAttributes(handle); err != nil || !supp {
		return nil, err
	}

	//Get the windows Extended Attributes using the file handle
	var extAtts []extendedAttribute
	extAtts, err := fgetEA(handle)
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

func (p *pathMetadataHandle) SecurityDescriptor() (buf *[]byte, err error) {
	f, err := openMetadataHandle(p.name, p.flag)
	if err != nil {
		return nil, err
	}
	defer func() {
		cerr := f.Close()
		if err == nil {
			err = cerr
		}
	}()

	return getSecurityDescriptor(windows.Handle(f.Fd()))
}

func (p *fdMetadataHandle) Xattr(_ bool) ([]restic.ExtendedAttribute, error) {
	return xattrFromHandle(p.name, windows.Handle(p.f.Fd()))
}

func (p *fdMetadataHandle) SecurityDescriptor() (*[]byte, error) {
	return getSecurityDescriptor(windows.Handle(p.f.Fd()))
}

func openMetadataHandle(path string, flag int) (*os.File, error) {
	// OpenFile from go does not request FILE_READ_EA so we need our own low-level implementation
	return openCustomHandle(path, flag, windows.FILE_READ_EA|windows.FILE_READ_ATTRIBUTES|windows.STANDARD_RIGHTS_READ)
}

func openReadHandle(path string, flag int) (*os.File, error) {
	return openCustomHandle(path, flag, windows.FILE_GENERIC_READ)
}

func openCustomHandle(path string, flag int, fileAccess int) (*os.File, error) {
	path = fixpath(path)
	// according to the windows docs, STANDARD_RIGHTS_READ (fileAccess) + FILE_FLAG_BACKUP_SEMANTICS disables security checks on access
	// if the process holds the SeBackupPrivilege
	shareMode := windows.FILE_SHARE_READ | windows.FILE_SHARE_WRITE | windows.FILE_SHARE_DELETE
	attrs := windows.FILE_ATTRIBUTE_NORMAL | windows.FILE_FLAG_BACKUP_SEMANTICS
	if flag&O_NOFOLLOW != 0 {
		attrs |= syscall.FILE_FLAG_OPEN_REPARSE_POINT
	}

	utf16Path := windows.StringToUTF16Ptr(path)
	handle, err := windows.CreateFile(utf16Path, uint32(fileAccess), uint32(shareMode), nil, windows.OPEN_EXISTING, uint32(attrs), 0)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(handle), path), nil
}

// reopenMetadataHandle reopens a handle created by openMetadataHandle for reading.
// The caller must no longer use the original file.
func reopenMetadataHandle(f *os.File) (*os.File, error) {
	defer func() {
		_ = f.Close()
	}()

	fileAccess := windows.FILE_GENERIC_READ
	shareMode := windows.FILE_SHARE_READ | windows.FILE_SHARE_WRITE | windows.FILE_SHARE_DELETE
	attrs := windows.FILE_ATTRIBUTE_NORMAL | windows.FILE_FLAG_BACKUP_SEMANTICS
	// FIXME correct FILE_FLAG_OPEN_REPARSE_POINT handling?

	handle, err := reOpenFile(windows.Handle(f.Fd()), uint32(fileAccess), uint32(shareMode), uint32(attrs))
	if err != nil {
		return nil, err
	}

	return os.NewFile(uintptr(handle), f.Name()), nil
}

var (
	modkernel32    = windows.NewLazySystemDLL("kernel32.dll")
	procReOpenFile = modkernel32.NewProc("ReOpenFile")
)

func reOpenFile(original windows.Handle, access uint32, mode uint32, attrs uint32) (handle windows.Handle, err error) {
	r0, _, e1 := syscall.SyscallN(procReOpenFile.Addr(), uintptr(original), uintptr(access), uintptr(mode), uintptr(attrs))
	handle = windows.Handle(r0)
	if handle == windows.InvalidHandle {
		err = errnoErr(e1)
	}
	return
}
