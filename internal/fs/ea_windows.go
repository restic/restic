//go:build windows
// +build windows

package fs

import (
	"fmt"
	"syscall"
	"unsafe"

	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"
)

// extendedAttribute is a type alias for winio.ExtendedAttribute
type extendedAttribute = winio.ExtendedAttribute

// encodeExtendedAttributes encodes the extended attributes to a byte slice.
func encodeExtendedAttributes(attrs []extendedAttribute) ([]byte, error) {
	return winio.EncodeExtendedAttributes(attrs)
}

// decodeExtendedAttributes decodes the extended attributes from a byte slice.
func decodeExtendedAttributes(data []byte) ([]extendedAttribute, error) {
	return winio.DecodeExtendedAttributes(data)
}

// The code below was copied over from https://github.com/microsoft/go-winio/blob/main/pipe.go under MIT license.

// The MIT License (MIT)

// Copyright (c) 2015 Microsoft

// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:

// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.

// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

type ntStatus int32

func (status ntStatus) Err() error {
	if status >= 0 {
		return nil
	}
	return rtlNtStatusToDosError(status)
}

// The code below was copied over from https://github.com/microsoft/go-winio/blob/main/zsyscall_windows.go under MIT license.

// ioStatusBlock represents the IO_STATUS_BLOCK struct defined here:
// https://docs.microsoft.com/en-us/windows-hardware/drivers/ddi/wdm/ns-wdm-_io_status_block
type ioStatusBlock struct {
	Status, Information uintptr
}

var (
	modntdll                       = windows.NewLazySystemDLL("ntdll.dll")
	procRtlNtStatusToDosErrorNoTeb = modntdll.NewProc("RtlNtStatusToDosErrorNoTeb")
)

func rtlNtStatusToDosError(status ntStatus) (winerr error) {
	r0, _, _ := syscall.SyscallN(procRtlNtStatusToDosErrorNoTeb.Addr(), uintptr(status))
	if r0 != 0 {
		winerr = syscall.Errno(r0)
	}
	return
}

// The code below was adapted from https://github.com/ambarve/go-winio/blob/a7564fd482feb903f9562a135f1317fd3b480739/ea.go
// under MIT license.

var (
	procNtQueryEaFile = modntdll.NewProc("NtQueryEaFile")
	procNtSetEaFile   = modntdll.NewProc("NtSetEaFile")
)

const (
	// STATUS_NO_EAS_ON_FILE is a constant value which indicates EAs were requested for the file but it has no EAs.
	// Windows NTSTATUS value: STATUS_NO_EAS_ON_FILE=0xC0000052
	STATUS_NO_EAS_ON_FILE = -1073741742
)

// fgetEA retrieves the extended attributes for the file represented by `handle`. The
// `handle` must have been opened with file access flag FILE_READ_EA (0x8).
// The extended file attribute names in windows are case-insensitive and when fetching
// the attributes the names are generally returned in UPPER case.
func fgetEA(handle windows.Handle) ([]extendedAttribute, error) {
	// default buffer size to start with
	bufLen := 1024
	buf := make([]byte, bufLen)
	var iosb ioStatusBlock
	// keep increasing the buffer size until it is large enough
	for {
		status := getFileEA(handle, &iosb, &buf[0], uint32(bufLen), false, 0, 0, nil, true)

		if status == STATUS_NO_EAS_ON_FILE {
			//If status is -1073741742, no extended attributes were found
			return nil, nil
		}
		err := status.Err()
		if err != nil {
			// convert ntstatus code to windows error
			if err == windows.ERROR_INSUFFICIENT_BUFFER || err == windows.ERROR_MORE_DATA {
				bufLen *= 2
				buf = make([]byte, bufLen)
				continue
			}
			return nil, fmt.Errorf("get file EA failed with: %w", err)
		}
		break
	}
	return decodeExtendedAttributes(buf)
}

// fsetEA sets the extended attributes for the file represented by `handle`.  The
// handle must have been opened with the file access flag FILE_WRITE_EA(0x10).
func fsetEA(handle windows.Handle, attrs []extendedAttribute) error {
	encodedEA, err := encodeExtendedAttributes(attrs)
	if err != nil {
		return fmt.Errorf("failed to encoded extended attributes: %w", err)
	}

	var iosb ioStatusBlock

	return setFileEA(handle, &iosb, &encodedEA[0], uint32(len(encodedEA))).Err()
}

// The code below was adapted from https://github.com/ambarve/go-winio/blob/a7564fd482feb903f9562a135f1317fd3b480739/zsyscall_windows.go
// under MIT license.

func getFileEA(handle windows.Handle, iosb *ioStatusBlock, buf *uint8, bufLen uint32, returnSingleEntry bool, eaList uintptr, eaListLen uint32, eaIndex *uint32, restartScan bool) (status ntStatus) {
	var _p0 uint32
	if returnSingleEntry {
		_p0 = 1
	}
	var _p1 uint32
	if restartScan {
		_p1 = 1
	}
	r0, _, _ := syscall.SyscallN(procNtQueryEaFile.Addr(), uintptr(handle), uintptr(unsafe.Pointer(iosb)), uintptr(unsafe.Pointer(buf)), uintptr(bufLen), uintptr(_p0), eaList, uintptr(eaListLen), uintptr(unsafe.Pointer(eaIndex)), uintptr(_p1))
	status = ntStatus(r0)
	return
}

func setFileEA(handle windows.Handle, iosb *ioStatusBlock, buf *uint8, bufLen uint32) (status ntStatus) {
	r0, _, _ := syscall.SyscallN(procNtSetEaFile.Addr(), uintptr(handle), uintptr(unsafe.Pointer(iosb)), uintptr(unsafe.Pointer(buf)), uintptr(bufLen))
	status = ntStatus(r0)
	return
}

// pathSupportsExtendedAttributes returns true if the path supports extended attributes.
func pathSupportsExtendedAttributes(path string) (supported bool, err error) {
	var fileSystemFlags uint32
	utf16Path, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return false, err
	}
	err = windows.GetVolumeInformation(utf16Path, nil, 0, nil, nil, &fileSystemFlags, nil, 0)
	if err != nil {
		return false, err
	}
	supported = (fileSystemFlags & windows.FILE_SUPPORTS_EXTENDED_ATTRIBUTES) != 0
	return supported, nil
}
