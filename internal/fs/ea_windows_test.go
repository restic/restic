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

// The code below was refactored from github.com/Microsoft/go-winio/blob/a7564fd482feb903f9562a135f1317fd3b480739/ea_test.go
// under MIT license.
func TestSetGetFileEA(t *testing.T) {
	testFilePath, testFile := setupTestFile(t)
	testEAs := generateTestEAs(t, 3, testFilePath)
	fileHandle := openFile(t, testFilePath, windows.FILE_ATTRIBUTE_NORMAL)
	defer closeFileHandle(t, testFilePath, testFile, fileHandle)

	testSetGetEA(t, testFilePath, fileHandle, testEAs)
}

// The code is new code and reuses code refactored from github.com/Microsoft/go-winio/blob/a7564fd482feb903f9562a135f1317fd3b480739/ea_test.go
// under MIT license.
func TestSetGetFolderEA(t *testing.T) {
	testFolderPath := setupTestFolder(t)

	testEAs := generateTestEAs(t, 3, testFolderPath)
	fileHandle := openFile(t, testFolderPath, windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_FLAG_BACKUP_SEMANTICS)
	defer closeFileHandle(t, testFolderPath, nil, fileHandle)

	testSetGetEA(t, testFolderPath, fileHandle, testEAs)
}

func setupTestFile(t *testing.T) (testFilePath string, testFile *os.File) {
	tempDir := t.TempDir()
	testFilePath = filepath.Join(tempDir, "testfile.txt")
	var err error
	if testFile, err = os.Create(testFilePath); err != nil {
		t.Fatalf("failed to create temporary file: %s", err)
	}
	return testFilePath, testFile
}

func setupTestFolder(t *testing.T) string {
	tempDir := t.TempDir()
	testfolderPath := filepath.Join(tempDir, "testfolder")
	if err := os.Mkdir(testfolderPath, os.ModeDir); err != nil {
		t.Fatalf("failed to create temporary folder: %s", err)
	}
	return testfolderPath
}

func generateTestEAs(t *testing.T, nAttrs int, path string) []ExtendedAttribute {
	testEAs := make([]ExtendedAttribute, nAttrs)
	for i := 0; i < nAttrs; i++ {
		testEAs[i].Name = fmt.Sprintf("TESTEA%d", i+1)
		testEAs[i].Value = make([]byte, getRandomInt())
		if _, err := rand.Read(testEAs[i].Value); err != nil {
			t.Logf("Error reading rand for path %s: %v\n", path, err)
		}
	}
	return testEAs
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

func openFile(t *testing.T, path string, attributes uint32) windows.Handle {
	utf16Path := windows.StringToUTF16Ptr(path)
	fileAccessRightReadWriteEA := uint32(0x8 | 0x10)
	fileHandle, err := windows.CreateFile(utf16Path, fileAccessRightReadWriteEA, 0, nil, windows.OPEN_EXISTING, attributes, 0)
	if err != nil {
		t.Fatalf("open file failed with: %s", err)
	}
	return fileHandle
}

func closeFileHandle(t *testing.T, testfilePath string, testFile *os.File, handle windows.Handle) {
	if testFile != nil {
		err := testFile.Close()
		if err != nil {
			t.Logf("Error closing file %s: %v\n", testFile.Name(), err)
		}
	}
	if err := windows.Close(handle); err != nil {
		t.Logf("Error closing file handle %s: %v\n", testfilePath, err)
	}
	cleanupTestFile(t, testfilePath)
}

func cleanupTestFile(t *testing.T, path string) {
	if err := os.Remove(path); err != nil {
		t.Logf("Error removing file/folder %s: %v\n", path, err)
	}
}

func testSetGetEA(t *testing.T, path string, handle windows.Handle, testEAs []ExtendedAttribute) {
	if err := SetFileEA(handle, testEAs); err != nil {
		t.Fatalf("set EA for path %s failed: %s", path, err)
	}

	readEAs, err := GetFileEA(handle)
	if err != nil {
		t.Fatalf("get EA for path %s failed: %s", path, err)
	}

	if !reflect.DeepEqual(readEAs, testEAs) {
		t.Logf("expected: %+v, found: %+v\n", testEAs, readEAs)
		t.Fatalf("EAs read from path %s don't match", path)
	}
}
