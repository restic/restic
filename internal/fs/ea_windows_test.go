//go:build windows
// +build windows

package fs

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"reflect"
	"syscall"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

// The code below was adapted from github.com/Microsoft/go-winio under MIT license.

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

// The code below was adapted from https://github.com/ambarve/go-winio/blob/a7564fd482feb903f9562a135f1317fd3b480739/ea_test.go
// under MIT license.

var (
	testEas = []ExtendedAttribute{
		{Name: "foo", Value: []byte("bar")},
		{Name: "fizz", Value: []byte("buzz")},
	}

	testEasEncoded = []byte{16, 0, 0, 0, 0, 3, 3, 0, 102, 111, 111, 0, 98, 97, 114, 0, 0,
		0, 0, 0, 0, 4, 4, 0, 102, 105, 122, 122, 0, 98, 117, 122, 122, 0, 0, 0}
	testEasNotPadded = testEasEncoded[0 : len(testEasEncoded)-3]
	testEasTruncated = testEasEncoded[0:20]
)

func TestRoundTripEas(t *testing.T) {
	b, err := EncodeExtendedAttributes(testEas)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(testEasEncoded, b) {
		t.Fatalf("Encoded mismatch %v %v", testEasEncoded, b)
	}
	eas, err := DecodeExtendedAttributes(b)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(testEas, eas) {
		t.Fatalf("mismatch %+v %+v", testEas, eas)
	}
}

func TestEasDontNeedPaddingAtEnd(t *testing.T) {
	eas, err := DecodeExtendedAttributes(testEasNotPadded)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(testEas, eas) {
		t.Fatalf("mismatch %+v %+v", testEas, eas)
	}
}

func TestTruncatedEasFailCorrectly(t *testing.T) {
	_, err := DecodeExtendedAttributes(testEasTruncated)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestNilEasEncodeAndDecodeAsNil(t *testing.T) {
	b, err := EncodeExtendedAttributes(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(b) != 0 {
		t.Fatal("expected empty")
	}
	eas, err := DecodeExtendedAttributes(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(eas) != 0 {
		t.Fatal("expected empty")
	}
}

// TestSetFileEa makes sure that the test buffer is actually parsable by NtSetEaFile.
func TestSetFileEa(t *testing.T) {
	f, err := os.CreateTemp("", "testea")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		err := os.Remove(f.Name())
		if err != nil {
			t.Logf("Error removing file %s: %v\n", f.Name(), err)
		}
		err = f.Close()
		if err != nil {
			t.Logf("Error closing file %s: %v\n", f.Name(), err)
		}
	}()
	ntdll := syscall.MustLoadDLL("ntdll.dll")
	ntSetEaFile := ntdll.MustFindProc("NtSetEaFile")
	var iosb [2]uintptr
	r, _, _ := ntSetEaFile.Call(f.Fd(),
		uintptr(unsafe.Pointer(&iosb[0])),
		uintptr(unsafe.Pointer(&testEasEncoded[0])),
		uintptr(len(testEasEncoded)))
	if r != 0 {
		t.Fatalf("NtSetEaFile failed with %08x", r)
	}
}

func TestSetGetFileEA(t *testing.T) {
	tempDir := t.TempDir()
	testfilePath := filepath.Join(tempDir, "testfile.txt")
	// create temp file
	testfile, err := os.Create(testfilePath)
	if err != nil {
		t.Fatalf("failed to create temporary file: %s", err)
	}
	defer func() {
		err := testfile.Close()
		if err != nil {
			t.Logf("Error closing file %s: %v\n", testfile.Name(), err)
		}
	}()

	nAttrs := 3
	testEAs := make([]ExtendedAttribute, 3)
	// generate random extended attributes for test
	for i := 0; i < nAttrs; i++ {
		// EA name is automatically converted to upper case before storing, so
		// when reading it back it returns the upper case name. To avoid test
		// failures because of that keep the name upper cased.
		testEAs[i].Name = fmt.Sprintf("TESTEA%d", i+1)
		testEAs[i].Value = make([]byte, getRandomInt())
		_, err := rand.Read(testEAs[i].Value)
		if err != nil {
			t.Logf("Error reading rand for file %s: %v\n", testfilePath, err)
		}
	}

	utf16Path := windows.StringToUTF16Ptr(testfilePath)
	fileAccessRightReadWriteEA := (0x8 | 0x10)
	fileHandle, err := windows.CreateFile(utf16Path, uint32(fileAccessRightReadWriteEA), 0, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatalf("open file failed with: %s", err)
	}
	defer func() {
		err := windows.Close(fileHandle)
		if err != nil {
			t.Logf("Error closing file handle %s: %v\n", testfilePath, err)
		}
	}()

	if err := SetFileEA(fileHandle, testEAs); err != nil {
		t.Fatalf("set EA for file failed: %s", err)
	}

	var readEAs []ExtendedAttribute
	if readEAs, err = GetFileEA(fileHandle); err != nil {
		t.Fatalf("get EA for file failed: %s", err)
	}

	if !reflect.DeepEqual(readEAs, testEAs) {
		t.Logf("expected: %+v, found: %+v\n", testEAs, readEAs)
		t.Fatalf("EAs read from testfile don't match")
	}
}

func TestSetGetFolderEA(t *testing.T) {
	tempDir := t.TempDir()
	testfolderPath := filepath.Join(tempDir, "testfolder")
	// create temp folder
	err := os.Mkdir(testfolderPath, os.ModeDir)
	if err != nil {
		t.Fatalf("failed to create temporary file: %s", err)
	}

	nAttrs := 3
	testEAs := make([]ExtendedAttribute, 3)
	// generate random extended attributes for test
	for i := 0; i < nAttrs; i++ {
		// EA name is automatically converted to upper case before storing, so
		// when reading it back it returns the upper case name. To avoid test
		// failures because of that keep the name upper cased.
		testEAs[i].Name = fmt.Sprintf("TESTEA%d", i+1)
		testEAs[i].Value = make([]byte, getRandomInt())
		_, err := rand.Read(testEAs[i].Value)
		if err != nil {
			t.Logf("Error reading rand for file %s: %v\n", testfolderPath, err)
		}
	}

	utf16Path := windows.StringToUTF16Ptr(testfolderPath)
	fileAccessRightReadWriteEA := (0x8 | 0x10)
	fileHandle, err := windows.CreateFile(utf16Path, uint32(fileAccessRightReadWriteEA), 0, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_FLAG_BACKUP_SEMANTICS, 0)

	if err != nil {
		t.Fatalf("open folder failed with: %s", err)
	}
	defer func() {
		err := windows.Close(fileHandle)
		if err != nil {
			t.Logf("Error closing file handle %s: %v\n", testfolderPath, err)
		}
	}()

	if err := SetFileEA(fileHandle, testEAs); err != nil {
		t.Fatalf("set EA for folder failed: %s", err)
	}

	var readEAs []ExtendedAttribute
	if readEAs, err = GetFileEA(fileHandle); err != nil {
		t.Fatalf("get EA for folder failed: %s", err)
	}

	if !reflect.DeepEqual(readEAs, testEAs) {
		t.Logf("expected: %+v, found: %+v\n", testEAs, readEAs)
		t.Fatalf("EAs read from test folder don't match")
	}
}

func getRandomInt() int64 {
	nBig, err := rand.Int(rand.Reader, big.NewInt(27))
	if err != nil {
		panic(err)
	}
	n := nBig.Int64()
	if n == 0 {
		n = getRandomInt()
	}
	return n
}
