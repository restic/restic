//go:build windows
// +build windows

package restorer

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"path"
	"path/filepath"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/test"
	rtest "github.com/restic/restic/internal/test"
	"golang.org/x/sys/windows"
)

func getBlockCount(t *testing.T, filename string) int64 {
	libkernel32 := windows.NewLazySystemDLL("kernel32.dll")
	err := libkernel32.Load()
	rtest.OK(t, err)
	proc := libkernel32.NewProc("GetCompressedFileSizeW")
	err = proc.Find()
	rtest.OK(t, err)

	namePtr, err := syscall.UTF16PtrFromString(filename)
	rtest.OK(t, err)

	result, _, _ := proc.Call(uintptr(unsafe.Pointer(namePtr)), 0)

	const invalidFileSize = uintptr(4294967295)
	if result == invalidFileSize {
		return -1
	}

	return int64(math.Ceil(float64(result) / 512))
}

type DataStreamInfo struct {
	name string
	data string
}

type NodeInfo struct {
	DataStreamInfo
	parentDir   string
	attributes  FileAttributes
	Exists      bool
	IsDirectory bool
}

func TestFileAttributeCombination(t *testing.T) {
	testFileAttributeCombination(t, false)
}

func TestEmptyFileAttributeCombination(t *testing.T) {
	testFileAttributeCombination(t, true)
}

func testFileAttributeCombination(t *testing.T, isEmpty bool) {
	t.Parallel()
	//Generate combination of 5 attributes.
	attributeCombinations := generateCombinations(5, []bool{})

	fileName := "TestFile.txt"
	// Iterate through each attribute combination
	for _, attr1 := range attributeCombinations {

		//Set up the required file information
		fileInfo := NodeInfo{
			DataStreamInfo: getDataStreamInfo(isEmpty, fileName),
			parentDir:      "dir",
			attributes:     getFileAttributes(attr1),
			Exists:         false,
		}

		//Get the current test name
		testName := getCombinationTestName(fileInfo, fileName, fileInfo.attributes)

		//Run test
		t.Run(testName, func(t *testing.T) {
			mainFilePath := runAttributeTests(t, fileInfo, fileInfo.attributes)

			verifyFileRestores(isEmpty, mainFilePath, t, fileInfo)
		})
	}
}

func generateCombinations(n int, prefix []bool) [][]bool {
	if n == 0 {
		// Return a slice containing the current permutation
		return [][]bool{append([]bool{}, prefix...)}
	}

	// Generate combinations with True
	prefixTrue := append(prefix, true)
	permsTrue := generateCombinations(n-1, prefixTrue)

	// Generate combinations with False
	prefixFalse := append(prefix, false)
	permsFalse := generateCombinations(n-1, prefixFalse)

	// Combine combinations with True and False
	return append(permsTrue, permsFalse...)
}

func getDataStreamInfo(isEmpty bool, fileName string) DataStreamInfo {
	var dataStreamInfo DataStreamInfo
	if isEmpty {
		dataStreamInfo = DataStreamInfo{
			name: fileName,
		}
	} else {
		dataStreamInfo = DataStreamInfo{
			name: fileName,
			data: "Main file data stream.",
		}
	}
	return dataStreamInfo
}

func getFileAttributes(values []bool) FileAttributes {
	return FileAttributes{
		ReadOnly:  values[0],
		Hidden:    values[1],
		System:    values[2],
		Archive:   values[3],
		Encrypted: values[4],
	}
}

func getCombinationTestName(fi NodeInfo, fileName string, overwriteAttr FileAttributes) string {
	if fi.attributes.ReadOnly {
		fileName += "-ReadOnly"
	}
	if fi.attributes.Hidden {
		fileName += "-Hidden"
	}
	if fi.attributes.System {
		fileName += "-System"
	}
	if fi.attributes.Archive {
		fileName += "-Archive"
	}
	if fi.attributes.Encrypted {
		fileName += "-Encrypted"
	}
	if fi.Exists {
		fileName += "-Overwrite"
		if overwriteAttr.ReadOnly {
			fileName += "-R"
		}
		if overwriteAttr.Hidden {
			fileName += "-H"
		}
		if overwriteAttr.System {
			fileName += "-S"
		}
		if overwriteAttr.Archive {
			fileName += "-A"
		}
		if overwriteAttr.Encrypted {
			fileName += "-E"
		}
	}
	return fileName
}

func runAttributeTests(t *testing.T, fileInfo NodeInfo, existingFileAttr FileAttributes) string {
	testDir := t.TempDir()
	res, _ := setupWithFileAttributes(t, fileInfo, testDir, existingFileAttr)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := res.RestoreTo(ctx, testDir)
	rtest.OK(t, err)

	mainFilePath := path.Join(testDir, fileInfo.parentDir, fileInfo.name)
	//Verify restore
	verifyFileAttributes(t, mainFilePath, fileInfo.attributes)
	return mainFilePath
}

func setupWithFileAttributes(t *testing.T, nodeInfo NodeInfo, testDir string, existingFileAttr FileAttributes) (*Restorer, []int) {
	t.Helper()
	if nodeInfo.Exists {
		if !nodeInfo.IsDirectory {
			err := os.MkdirAll(path.Join(testDir, nodeInfo.parentDir), os.ModeDir)
			rtest.OK(t, err)
			filepath := path.Join(testDir, nodeInfo.parentDir, nodeInfo.name)
			if existingFileAttr.Encrypted {
				err := createEncryptedFileWriteData(filepath, nodeInfo)
				rtest.OK(t, err)
			} else {
				// Write the data to the file
				file, err := os.OpenFile(path.Clean(filepath), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
				rtest.OK(t, err)
				_, err = file.Write([]byte(nodeInfo.data))
				rtest.OK(t, err)

				err = file.Close()
				rtest.OK(t, err)
			}
		} else {
			err := os.MkdirAll(path.Join(testDir, nodeInfo.parentDir, nodeInfo.name), os.ModeDir)
			rtest.OK(t, err)
		}

		pathPointer, err := syscall.UTF16PtrFromString(path.Join(testDir, nodeInfo.parentDir, nodeInfo.name))
		rtest.OK(t, err)
		syscall.SetFileAttributes(pathPointer, getAttributeValue(&existingFileAttr))
	}

	index := 0

	order := []int{}
	streams := []DataStreamInfo{}
	if !nodeInfo.IsDirectory {
		order = append(order, index)
		index++
		streams = append(streams, nodeInfo.DataStreamInfo)
	}
	return setup(t, getNodes(nodeInfo.parentDir, nodeInfo.name, order, streams, nodeInfo.IsDirectory, &nodeInfo.attributes)), order
}

func createEncryptedFileWriteData(filepath string, fileInfo NodeInfo) (err error) {
	var ptr *uint16
	if ptr, err = windows.UTF16PtrFromString(filepath); err != nil {
		return err
	}
	var handle windows.Handle
	//Create the file with encrypted flag
	if handle, err = windows.CreateFile(ptr, uint32(windows.GENERIC_READ|windows.GENERIC_WRITE), uint32(windows.FILE_SHARE_READ), nil, uint32(windows.CREATE_ALWAYS), windows.FILE_ATTRIBUTE_ENCRYPTED, 0); err != nil {
		return err
	}
	//Write data to file
	if _, err = windows.Write(handle, []byte(fileInfo.data)); err != nil {
		return err
	}
	//Close handle
	return windows.CloseHandle(handle)
}

func setup(t *testing.T, nodesMap map[string]Node) *Restorer {
	repo := repository.TestRepository(t)
	getFileAttributes := func(attr *FileAttributes, isDir bool) (genericAttributes map[restic.GenericAttributeType]json.RawMessage) {
		if attr == nil {
			return
		}

		fileattr := getAttributeValue(attr)

		if isDir {
			//If the node is a directory add FILE_ATTRIBUTE_DIRECTORY to attributes
			fileattr |= windows.FILE_ATTRIBUTE_DIRECTORY
		}
		attrs, err := restic.WindowsAttrsToGenericAttributes(restic.WindowsAttributes{FileAttributes: &fileattr})
		test.OK(t, err)
		return attrs
	}
	sn, _ := saveSnapshot(t, repo, Snapshot{
		Nodes: nodesMap,
	}, getFileAttributes)
	res := NewRestorer(repo, sn, Options{})
	return res
}

func getAttributeValue(attr *FileAttributes) uint32 {
	var fileattr uint32
	if attr.ReadOnly {
		fileattr |= windows.FILE_ATTRIBUTE_READONLY
	}
	if attr.Hidden {
		fileattr |= windows.FILE_ATTRIBUTE_HIDDEN
	}
	if attr.Encrypted {
		fileattr |= windows.FILE_ATTRIBUTE_ENCRYPTED
	}
	if attr.Archive {
		fileattr |= windows.FILE_ATTRIBUTE_ARCHIVE
	}
	if attr.System {
		fileattr |= windows.FILE_ATTRIBUTE_SYSTEM
	}
	return fileattr
}

func getNodes(dir string, mainNodeName string, order []int, streams []DataStreamInfo, isDirectory bool, attributes *FileAttributes) map[string]Node {
	var mode os.FileMode
	if isDirectory {
		mode = os.FileMode(2147484159)
	} else {
		if attributes != nil && attributes.ReadOnly {
			mode = os.FileMode(0o444)
		} else {
			mode = os.FileMode(0o666)
		}
	}

	getFileNodes := func() map[string]Node {
		nodes := map[string]Node{}
		if isDirectory {
			//Add a directory node at the same level as the other streams
			nodes[mainNodeName] = Dir{
				ModTime:    time.Now(),
				attributes: attributes,
				Mode:       mode,
			}
		}

		if len(streams) > 0 {
			for _, index := range order {
				stream := streams[index]

				var attr *FileAttributes = nil
				if mainNodeName == stream.name {
					attr = attributes
				} else if attributes != nil && attributes.Encrypted {
					//Set encrypted attribute
					attr = &FileAttributes{Encrypted: true}
				}

				nodes[stream.name] = File{
					ModTime:    time.Now(),
					Data:       stream.data,
					Mode:       mode,
					attributes: attr,
				}
			}
		}
		return nodes
	}

	return map[string]Node{
		dir: Dir{
			Mode:    normalizeFileMode(0750 | mode),
			ModTime: time.Now(),
			Nodes:   getFileNodes(),
		},
	}
}

func verifyFileAttributes(t *testing.T, mainFilePath string, attr FileAttributes) {
	ptr, err := windows.UTF16PtrFromString(mainFilePath)
	rtest.OK(t, err)
	//Get file attributes using syscall
	fileAttributes, err := syscall.GetFileAttributes(ptr)
	rtest.OK(t, err)
	//Test positive and negative scenarios
	if attr.ReadOnly {
		rtest.Assert(t, fileAttributes&windows.FILE_ATTRIBUTE_READONLY != 0, "Expected read only attribute.")
	} else {
		rtest.Assert(t, fileAttributes&windows.FILE_ATTRIBUTE_READONLY == 0, "Unexpected read only attribute.")
	}
	if attr.Hidden {
		rtest.Assert(t, fileAttributes&windows.FILE_ATTRIBUTE_HIDDEN != 0, "Expected hidden attribute.")
	} else {
		rtest.Assert(t, fileAttributes&windows.FILE_ATTRIBUTE_HIDDEN == 0, "Unexpected hidden attribute.")
	}
	if attr.System {
		rtest.Assert(t, fileAttributes&windows.FILE_ATTRIBUTE_SYSTEM != 0, "Expected system attribute.")
	} else {
		rtest.Assert(t, fileAttributes&windows.FILE_ATTRIBUTE_SYSTEM == 0, "Unexpected system attribute.")
	}
	if attr.Archive {
		rtest.Assert(t, fileAttributes&windows.FILE_ATTRIBUTE_ARCHIVE != 0, "Expected archive attribute.")
	} else {
		rtest.Assert(t, fileAttributes&windows.FILE_ATTRIBUTE_ARCHIVE == 0, "Unexpected archive attribute.")
	}
	if attr.Encrypted {
		rtest.Assert(t, fileAttributes&windows.FILE_ATTRIBUTE_ENCRYPTED != 0, "Expected encrypted attribute.")
	} else {
		rtest.Assert(t, fileAttributes&windows.FILE_ATTRIBUTE_ENCRYPTED == 0, "Unexpected encrypted attribute.")
	}
}

func verifyFileRestores(isEmpty bool, mainFilePath string, t *testing.T, fileInfo NodeInfo) {
	if isEmpty {
		_, err1 := os.Stat(mainFilePath)
		rtest.Assert(t, !errors.Is(err1, os.ErrNotExist), "The file "+fileInfo.name+" does not exist")
	} else {

		verifyMainFileRestore(t, mainFilePath, fileInfo)
	}
}

func verifyMainFileRestore(t *testing.T, mainFilePath string, fileInfo NodeInfo) {
	fi, err1 := os.Stat(mainFilePath)
	rtest.Assert(t, !errors.Is(err1, os.ErrNotExist), "The file "+fileInfo.name+" does not exist")

	size := fi.Size()
	rtest.Assert(t, size > 0, "The file "+fileInfo.name+" exists but is empty")

	content, err := os.ReadFile(mainFilePath)
	rtest.OK(t, err)
	rtest.Assert(t, string(content) == fileInfo.data, "The file "+fileInfo.name+" exists but the content is not overwritten")
}

func TestDirAttributeCombination(t *testing.T) {
	t.Parallel()
	attributeCombinations := generateCombinations(4, []bool{})

	dirName := "TestDir"
	// Iterate through each attribute combination
	for _, attr1 := range attributeCombinations {

		//Set up the required directory information
		dirInfo := NodeInfo{
			DataStreamInfo: DataStreamInfo{
				name: dirName,
			},
			parentDir:   "dir",
			attributes:  getDirFileAttributes(attr1),
			Exists:      false,
			IsDirectory: true,
		}

		//Get the current test name
		testName := getCombinationTestName(dirInfo, dirName, dirInfo.attributes)

		//Run test
		t.Run(testName, func(t *testing.T) {
			mainDirPath := runAttributeTests(t, dirInfo, dirInfo.attributes)

			//Check directory exists
			_, err1 := os.Stat(mainDirPath)
			rtest.Assert(t, !errors.Is(err1, os.ErrNotExist), "The directory "+dirInfo.name+" does not exist")
		})
	}
}

func getDirFileAttributes(values []bool) FileAttributes {
	return FileAttributes{
		// readonly not valid for directories
		Hidden:    values[0],
		System:    values[1],
		Archive:   values[2],
		Encrypted: values[3],
	}
}

func TestFileAttributeCombinationsOverwrite(t *testing.T) {
	testFileAttributeCombinationsOverwrite(t, false)
}

func TestEmptyFileAttributeCombinationsOverwrite(t *testing.T) {
	testFileAttributeCombinationsOverwrite(t, true)
}

func testFileAttributeCombinationsOverwrite(t *testing.T, isEmpty bool) {
	t.Parallel()
	//Get attribute combinations
	attributeCombinations := generateCombinations(5, []bool{})
	//Get overwrite file attribute combinations
	overwriteCombinations := generateCombinations(5, []bool{})

	fileName := "TestOverwriteFile"

	//Iterate through each attribute combination
	for _, attr1 := range attributeCombinations {

		fileInfo := NodeInfo{
			DataStreamInfo: getDataStreamInfo(isEmpty, fileName),
			parentDir:      "dir",
			attributes:     getFileAttributes(attr1),
			Exists:         true,
		}

		overwriteFileAttributes := []FileAttributes{}

		for _, overwrite := range overwriteCombinations {
			overwriteFileAttributes = append(overwriteFileAttributes, getFileAttributes(overwrite))
		}

		//Iterate through each overwrite attribute combination
		for _, overwriteFileAttr := range overwriteFileAttributes {
			//Get the test name
			testName := getCombinationTestName(fileInfo, fileName, overwriteFileAttr)

			//Run test
			t.Run(testName, func(t *testing.T) {
				mainFilePath := runAttributeTests(t, fileInfo, overwriteFileAttr)

				verifyFileRestores(isEmpty, mainFilePath, t, fileInfo)
			})
		}
	}
}

func TestDirAttributeCombinationsOverwrite(t *testing.T) {
	t.Parallel()
	//Get attribute combinations
	attributeCombinations := generateCombinations(4, []bool{})
	//Get overwrite dir attribute combinations
	overwriteCombinations := generateCombinations(4, []bool{})

	dirName := "TestOverwriteDir"

	//Iterate through each attribute combination
	for _, attr1 := range attributeCombinations {

		dirInfo := NodeInfo{
			DataStreamInfo: DataStreamInfo{
				name: dirName,
			},
			parentDir:   "dir",
			attributes:  getDirFileAttributes(attr1),
			Exists:      true,
			IsDirectory: true,
		}

		overwriteDirFileAttributes := []FileAttributes{}

		for _, overwrite := range overwriteCombinations {
			overwriteDirFileAttributes = append(overwriteDirFileAttributes, getDirFileAttributes(overwrite))
		}

		//Iterate through each overwrite attribute combinations
		for _, overwriteDirAttr := range overwriteDirFileAttributes {
			//Get the test name
			testName := getCombinationTestName(dirInfo, dirName, overwriteDirAttr)

			//Run test
			t.Run(testName, func(t *testing.T) {
				mainDirPath := runAttributeTests(t, dirInfo, dirInfo.attributes)

				//Check directory exists
				_, err1 := os.Stat(mainDirPath)
				rtest.Assert(t, !errors.Is(err1, os.ErrNotExist), "The directory "+dirInfo.name+" does not exist")
			})
		}
	}
}

func TestRestoreDeleteCaseInsensitive(t *testing.T) {
	repo := repository.TestRepository(t)
	tempdir := rtest.TempDir(t)

	sn, _ := saveSnapshot(t, repo, Snapshot{
		Nodes: map[string]Node{
			"anotherfile": File{Data: "content: file\n"},
		},
	}, noopGetGenericAttributes)

	// should delete files that no longer exist in the snapshot
	deleteSn, _ := saveSnapshot(t, repo, Snapshot{
		Nodes: map[string]Node{
			"AnotherfilE": File{Data: "content: file\n"},
		},
	}, noopGetGenericAttributes)

	res := NewRestorer(repo, sn, Options{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := res.RestoreTo(ctx, tempdir)
	rtest.OK(t, err)

	res = NewRestorer(repo, deleteSn, Options{Delete: true})
	err = res.RestoreTo(ctx, tempdir)
	rtest.OK(t, err)

	// anotherfile must still exist
	_, err = os.Stat(filepath.Join(tempdir, "anotherfile"))
	rtest.OK(t, err)
}
