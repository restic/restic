//go:build windows
// +build windows

package restorer

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"path"
	"strings"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
	restoreui "github.com/restic/restic/internal/ui/restore"
	"golang.org/x/sys/windows"
)

// Index of the main file stream for testing streams when the restoration order is different.
// This is mainly used to test scenarios in which the main file stream is restored after restoring another stream.
// Handling this scenario is important as creating the main file will usually replace the existing streams.
// '-1' is used because this will allow us to handle the ads stream indexes as they are. We just need to insert the main file index in the place we want it to be restored.
const MAIN_STREAM_ORDER_INDEX = -1

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

type NodeTestInfo struct {
	DataStreamInfo //The main data stream of the file
	parentDir      string
	attributes     *FileAttributes
	IsDirectory    bool
	//The order for restoration of streams in Ads streams
	//We also includes main stream index (-1) to the order to indcate when the main file should be restored.
	StreamRestoreOrder []int
	AdsStreams         []DataStreamInfo //Alternate streams of the node
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
	testAttributeCombinations(t, attributeCombinations, fileName, isEmpty, false, false, NodeTestInfo{})
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

func testAttributeCombinations(t *testing.T, attributeCombinations [][]bool, nodeName string, isEmpty, isDirectory, createExisting bool, existingNode NodeTestInfo) {
	// Iterate through each attribute combination
	for _, attr1 := range attributeCombinations {

		//Set up the node that needs to be restored
		nodeInfo := NodeTestInfo{
			DataStreamInfo: getDummyDataStream(isEmpty || isDirectory, nodeName, false),
			parentDir:      "dir",
			attributes:     convertToFileAttributes(attr1, isDirectory),
			IsDirectory:    isDirectory,
		}

		//Get the current test name
		testName := getCombinationTestName(nodeInfo, nodeName, createExisting, existingNode)

		//Run test
		t.Run(testName, func(t *testing.T) {

			// run the test and verify attributes
			mainPath := runAttributeTests(t, nodeInfo, createExisting, existingNode)

			//verify node restoration
			verifyRestores(t, isEmpty || isDirectory, mainPath, nodeInfo.DataStreamInfo)
		})
	}
}

func getDummyDataStream(isEmptyOrDirectory bool, mainStreamName string, isExisting bool) DataStreamInfo {
	var dataStreamInfo DataStreamInfo

	// Set only the name if the node is empty or is a directory.
	if isEmptyOrDirectory {
		dataStreamInfo = DataStreamInfo{
			name: mainStreamName,
		}
	} else {
		data := "Main file data stream."
		if isExisting {
			//Use a differnt data for existing files
			data = "Existing file data"
		}
		dataStreamInfo = DataStreamInfo{
			name: mainStreamName,
			data: data,
		}
	}
	return dataStreamInfo
}

// Convert boolean values to file attributes
func convertToFileAttributes(values []bool, isDirectory bool) *FileAttributes {
	if isDirectory {
		return &FileAttributes{
			// readonly not valid for directories
			Hidden:    values[0],
			System:    values[1],
			Archive:   values[2],
			Encrypted: values[3],
		}
	}

	return &FileAttributes{
		ReadOnly:  values[0],
		Hidden:    values[1],
		System:    values[2],
		Archive:   values[3],
		Encrypted: values[4],
	}
}

// generate name for the provide attribute combination
func getCombinationTestName(fi NodeTestInfo, fileName string, createExisiting bool, existingNode NodeTestInfo) string {
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
	if !createExisiting {
		return fileName
	}

	// Additonal name for existing file attributes test
	fileName += "-Overwrite"
	if existingNode.attributes.ReadOnly {
		fileName += "-R"
	}
	if existingNode.attributes.Hidden {
		fileName += "-H"
	}
	if existingNode.attributes.System {
		fileName += "-S"
	}
	if existingNode.attributes.Archive {
		fileName += "-A"
	}
	if existingNode.attributes.Encrypted {
		fileName += "-E"
	}
	return fileName
}

func runAttributeTests(t *testing.T, fileInfo NodeTestInfo, createExisting bool, existingNodeInfo NodeTestInfo) string {
	testDir := t.TempDir()
	runRestorerTest(t, fileInfo, testDir, createExisting, existingNodeInfo)

	mainFilePath := path.Join(testDir, fileInfo.parentDir, fileInfo.name)
	verifyAttributes(t, mainFilePath, fileInfo.attributes)
	return mainFilePath
}

func runRestorerTest(t *testing.T, nodeInfo NodeTestInfo, testDir string, createExisting bool, existingNodeInfo NodeTestInfo) {
	res := setup(t, nodeInfo, testDir, createExisting, existingNodeInfo)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_, err := res.RestoreTo(ctx, testDir)
	rtest.OK(t, err)
}

func setup(t *testing.T, nodeInfo NodeTestInfo, testDir string, createExisitingFile bool, existingNodeInfo NodeTestInfo) *Restorer {
	t.Helper()
	if createExisitingFile {
		createExisting(t, testDir, existingNodeInfo)
	}

	if !nodeInfo.IsDirectory && nodeInfo.StreamRestoreOrder == nil {
		nodeInfo.StreamRestoreOrder = []int{MAIN_STREAM_ORDER_INDEX}
	}

	nodesMap := getNodes(nodeInfo.parentDir, nodeInfo)

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
		rtest.OK(t, err)
		return attrs
	}

	getAdsAttributes := func(path string, hasAds, isAds bool) map[restic.GenericAttributeType]json.RawMessage {
		if isAds {
			windowsAttr := restic.WindowsAttributes{
				IsADS: &isAds,
			}
			attrs, err := restic.WindowsAttrsToGenericAttributes(windowsAttr)
			rtest.OK(t, err)
			return attrs
		} else if hasAds {
			//Find ads names by recursively searching through nodes
			//This is needed when multiple levels of parent directories are defined for ads file
			adsNames := findAdsNamesRecursively(nodesMap, path, []string{})
			windowsAttr := restic.WindowsAttributes{
				HasADS: &adsNames,
			}
			attrs, err := restic.WindowsAttrsToGenericAttributes(windowsAttr)
			rtest.OK(t, err)
			return attrs
		} else {
			return map[restic.GenericAttributeType]json.RawMessage{}
		}
	}

	repo := repository.TestRepository(t)
	sn, _ := saveSnapshot(t, repo, Snapshot{
		Nodes: nodesMap,
	}, getFileAttributes, getAdsAttributes)

	mock := &printerMock{}
	progress := restoreui.NewProgress(mock, 0)
	res := NewRestorer(repo, sn, Options{Progress: progress})
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

func createExisting(t *testing.T, testDir string, nodeInfo NodeTestInfo) {
	//Create directory or file for testing with node already exist in the folder.
	if !nodeInfo.IsDirectory {
		err := os.MkdirAll(path.Join(testDir, nodeInfo.parentDir), os.ModeDir)
		rtest.OK(t, err)

		filepath := path.Join(testDir, nodeInfo.parentDir, nodeInfo.name)
		createTestFile(t, nodeInfo.attributes.Encrypted, filepath, nodeInfo.DataStreamInfo)
	} else {
		err := os.MkdirAll(path.Join(testDir, nodeInfo.parentDir, nodeInfo.name), os.ModeDir)
		rtest.OK(t, err)
	}

	//Create ads streams if any
	if len(nodeInfo.AdsStreams) > 0 {
		for _, stream := range nodeInfo.AdsStreams {
			filepath := path.Join(testDir, nodeInfo.parentDir, stream.name)
			createTestFile(t, nodeInfo.attributes.Encrypted, filepath, stream)
		}
	}

	//Set attributes
	pathPointer, err := syscall.UTF16PtrFromString(path.Join(testDir, nodeInfo.parentDir, nodeInfo.name))
	rtest.OK(t, err)

	syscall.SetFileAttributes(pathPointer, getAttributeValue(nodeInfo.attributes))
}

func createTestFile(t *testing.T, isEncrypted bool, filepath string, stream DataStreamInfo) {

	var attribute uint32 = windows.FILE_ATTRIBUTE_NORMAL
	if isEncrypted {
		attribute = windows.FILE_ATTRIBUTE_ENCRYPTED
	}

	var ptr *uint16
	ptr, err := windows.UTF16PtrFromString(filepath)
	rtest.OK(t, err)

	//Create the file with attribute flag
	handle, err := windows.CreateFile(ptr, uint32(windows.GENERIC_READ|windows.GENERIC_WRITE), uint32(windows.FILE_SHARE_READ), nil, uint32(windows.CREATE_ALWAYS), attribute, 0)
	rtest.OK(t, err)

	//Write data to file
	_, err = windows.Write(handle, []byte(stream.data))
	rtest.OK(t, err)

	//Close handle
	rtest.OK(t, windows.CloseHandle(handle))
}

func getNodes(dir string, node NodeTestInfo) map[string]Node {
	var mode os.FileMode
	if node.IsDirectory {
		mode = os.FileMode(2147484159)
	} else {
		if node.attributes != nil && node.attributes.ReadOnly {
			mode = os.FileMode(0o444)
		} else {
			mode = os.FileMode(0o666)
		}
	}

	getFileNodes := func() map[string]Node {
		nodes := map[string]Node{}
		if node.IsDirectory {
			//Add a directory node at the same level as the other streams
			nodes[node.name] = Dir{
				ModTime:    time.Now(),
				hasAds:     len(node.AdsStreams) > 1,
				attributes: node.attributes,
				Mode:       mode,
			}
		}

		// Add nodes to the node map in the order we want.
		// This ensures the restoration of nodes in the specific order.
		for _, index := range node.StreamRestoreOrder {
			if index == MAIN_STREAM_ORDER_INDEX && !node.IsDirectory {
				//If main file then use the data stream from nodeinfo
				nodes[node.DataStreamInfo.name] = File{
					ModTime:    time.Now(),
					Data:       node.DataStreamInfo.data,
					Mode:       mode,
					attributes: node.attributes,
					hasAds:     len(node.AdsStreams) > 1,
					isAds:      false,
				}
			} else {
				//Else take the node from the AdsStreams of the node
				attr := &FileAttributes{}
				if node.attributes != nil && node.attributes.Encrypted {
					//Setting the encrypted attribute for ads streams.
					//This is needed when an encrypted ads stream is restored first, we need to create the file with encrypted attribute.
					attr.Encrypted = true
				}

				nodes[node.AdsStreams[index].name] = File{
					ModTime:    time.Now(),
					Data:       node.AdsStreams[index].data,
					Mode:       mode,
					attributes: attr,
					hasAds:     false,
					isAds:      true,
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

func findAdsNamesRecursively(nodesMap map[string]Node, path string, adsNames []string) []string {
	for name, node := range nodesMap {
		if restic.TrimAds(name) == path && name != path {
			adsNames = append(adsNames, strings.Replace(name, path, "", -1))
		} else if dir, ok := node.(Dir); ok && len(dir.Nodes) > 0 {
			adsNames = findAdsNamesRecursively(dir.Nodes, path, adsNames)
		}
	}
	return adsNames
}

func verifyAttributes(t *testing.T, mainFilePath string, attr *FileAttributes) {
	ptr, err := windows.UTF16PtrFromString(mainFilePath)
	rtest.OK(t, err)

	fileAttributes, err := syscall.GetFileAttributes(ptr)
	rtest.OK(t, err)
	//Test positive and negative scenarios
	if attr.ReadOnly {
		rtest.Assert(t, fileAttributes&windows.FILE_ATTRIBUTE_READONLY != 0, "Expected read only attibute.")
	} else {
		rtest.Assert(t, fileAttributes&windows.FILE_ATTRIBUTE_READONLY == 0, "Unexpected read only attibute.")
	}
	if attr.Hidden {
		rtest.Assert(t, fileAttributes&windows.FILE_ATTRIBUTE_HIDDEN != 0, "Expected hidden attibute.")
	} else {
		rtest.Assert(t, fileAttributes&windows.FILE_ATTRIBUTE_HIDDEN == 0, "Unexpected hidden attibute.")
	}
	if attr.System {
		rtest.Assert(t, fileAttributes&windows.FILE_ATTRIBUTE_SYSTEM != 0, "Expected system attibute.")
	} else {
		rtest.Assert(t, fileAttributes&windows.FILE_ATTRIBUTE_SYSTEM == 0, "Unexpected system attibute.")
	}
	if attr.Archive {
		rtest.Assert(t, fileAttributes&windows.FILE_ATTRIBUTE_ARCHIVE != 0, "Expected archive attibute.")
	} else {
		rtest.Assert(t, fileAttributes&windows.FILE_ATTRIBUTE_ARCHIVE == 0, "Unexpected archive attibute.")
	}
	if attr.Encrypted {
		rtest.Assert(t, fileAttributes&windows.FILE_ATTRIBUTE_ENCRYPTED != 0, "Expected encrypted attibute.")
	} else {
		rtest.Assert(t, fileAttributes&windows.FILE_ATTRIBUTE_ENCRYPTED == 0, "Unexpected encrypted attibute.")
	}
}

func verifyRestores(t *testing.T, isEmptyOrDirectory bool, path string, dsInfo DataStreamInfo) {
	fi, err1 := os.Stat(path)
	rtest.Assert(t, !errors.Is(err1, os.ErrNotExist), "The node "+dsInfo.name+" does not exist")

	//If the node is not a directoru or should not be empty, check its contents.
	if !isEmptyOrDirectory {
		size := fi.Size()
		rtest.Assert(t, size > 0, "The file "+dsInfo.name+" exists but is empty")

		content, err := os.ReadFile(path)
		rtest.OK(t, err)
		rtest.Assert(t, string(content) == dsInfo.data, "The file "+dsInfo.name+" exists but the content is not overwritten")
	}
}

func TestDirAttributeCombination(t *testing.T) {
	t.Parallel()
	attributeCombinations := generateCombinations(4, []bool{})

	dirName := "TestDir"
	testAttributeCombinations(t, attributeCombinations, dirName, false, true, false, NodeTestInfo{})
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
	//Get existing file attribute combinations
	overwriteCombinations := generateCombinations(5, []bool{})

	fileName := "TestOverwriteFile"

	testAttributeCombinationsOverwrite(t, attributeCombinations, overwriteCombinations, isEmpty, fileName, false)
}

func testAttributeCombinationsOverwrite(t *testing.T, attributeCombinations [][]bool, overwriteCombinations [][]bool, isEmpty bool, nodeName string, isDirectory bool) {
	// Convert existing attributes boolean value combinations to FileAttributes list
	existingFileAttribute := []FileAttributes{}
	for _, overwrite := range overwriteCombinations {
		existingFileAttribute = append(existingFileAttribute, *convertToFileAttributes(overwrite, isDirectory))
	}

	//Iterate through each existing attribute combination
	for _, existingFileAttr := range existingFileAttribute {
		exisitngNodeInfo := NodeTestInfo{
			DataStreamInfo: getDummyDataStream(isEmpty || isDirectory, nodeName, true),
			parentDir:      "dir",
			attributes:     &existingFileAttr,
			IsDirectory:    isDirectory,
		}

		testAttributeCombinations(t, attributeCombinations, nodeName, isEmpty, isDirectory, true, exisitngNodeInfo)
	}
}

func TestDirAttributeCombinationsOverwrite(t *testing.T) {
	t.Parallel()
	//Get attribute combinations
	attributeCombinations := generateCombinations(4, []bool{})
	//Get existing dir attribute combinations
	overwriteCombinations := generateCombinations(4, []bool{})

	dirName := "TestOverwriteDir"

	testAttributeCombinationsOverwrite(t, attributeCombinations, overwriteCombinations, true, dirName, true)
}

func TestOrderedAdsFile(t *testing.T) {
	dataStreams := []DataStreamInfo{
		{"OrderedAdsFile.text:datastream1:$DATA", "First data stream."},
		{"OrderedAdsFile.text:datastream2:$DATA", "Second data stream."},
	}

	var tests = map[string]struct {
		fileOrder []int
		Exists    bool
	}{
		"main-stream-first": {
			fileOrder: []int{MAIN_STREAM_ORDER_INDEX, 0, 1},
		},
		"second-stream-first": {
			fileOrder: []int{0, MAIN_STREAM_ORDER_INDEX, 1},
		},
		"main-stream-first-already-exists": {
			fileOrder: []int{MAIN_STREAM_ORDER_INDEX, 0, 1},
			Exists:    true,
		},
		"second-stream-first-already-exists": {
			fileOrder: []int{0, MAIN_STREAM_ORDER_INDEX, 1},
			Exists:    true,
		},
	}

	mainStreamName := "OrderedAdsFile.text"
	dir := "dir"

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			tempdir := rtest.TempDir(t)

			nodeInfo := NodeTestInfo{
				parentDir:          dir,
				attributes:         &FileAttributes{},
				DataStreamInfo:     getDummyDataStream(false, mainStreamName, false),
				StreamRestoreOrder: test.fileOrder,
				AdsStreams:         dataStreams,
			}

			exisitingNode := NodeTestInfo{}
			if test.Exists {
				exisitingNode = NodeTestInfo{
					parentDir:          dir,
					attributes:         &FileAttributes{},
					DataStreamInfo:     getDummyDataStream(false, mainStreamName, true),
					StreamRestoreOrder: test.fileOrder,
					AdsStreams:         dataStreams,
				}
			}

			runRestorerTest(t, nodeInfo, tempdir, test.Exists, exisitingNode)
			verifyRestoreOrder(t, nodeInfo, tempdir)
		})
	}
}

func verifyRestoreOrder(t *testing.T, nodeInfo NodeTestInfo, tempdir string) {
	for _, fileIndex := range nodeInfo.StreamRestoreOrder {

		var stream DataStreamInfo
		if fileIndex == MAIN_STREAM_ORDER_INDEX {
			stream = nodeInfo.DataStreamInfo
		} else {
			stream = nodeInfo.AdsStreams[fileIndex]
		}

		fp := path.Join(tempdir, nodeInfo.parentDir, stream.name)
		verifyRestores(t, false, fp, stream)
	}
}

func TestExistingStreamRemoval(t *testing.T) {
	tempdir := rtest.TempDir(t)
	dirName := "dir"
	mainFileName := "TestExistingStream.text"

	existingFileStreams := []DataStreamInfo{
		{"TestExistingStream.text:datastream1:$DATA", "Existing stream 1."},
		{"TestExistingStream.text:datastream2:$DATA", "Existing stream 2."},
		{"TestExistingStream.text:datastream3:$DATA", "Existing stream 3."},
		{"TestExistingStream.text:datastream4:$DATA", "Existing stream 4."},
	}

	restoringStreams := []DataStreamInfo{
		{"TestExistingStream.text:datastream1:$DATA", "First data stream."},
		{"TestExistingStream.text:datastream2:$DATA", "Second data stream."},
	}

	nodeInfo := NodeTestInfo{
		parentDir:  dirName,
		attributes: &FileAttributes{},
		DataStreamInfo: DataStreamInfo{
			name: mainFileName,
			data: "Main file data.",
		},
		StreamRestoreOrder: []int{MAIN_STREAM_ORDER_INDEX, 0, 1},
		AdsStreams:         restoringStreams,
	}

	existingNodeInfo := NodeTestInfo{
		parentDir:  dirName,
		attributes: &FileAttributes{},
		DataStreamInfo: DataStreamInfo{
			name: mainFileName,
			data: "Existing main stream.",
		},
		StreamRestoreOrder: []int{MAIN_STREAM_ORDER_INDEX, 0, 1, 2, 3, 4},
		AdsStreams:         existingFileStreams}

	runRestorerTest(t, nodeInfo, tempdir, true, existingNodeInfo)
	verifyExistingStreamRemoval(t, existingFileStreams, tempdir, dirName, restoringStreams)

	dirPath := path.Join(tempdir, nodeInfo.parentDir, nodeInfo.name)
	verifyRestores(t, true, dirPath, nodeInfo.DataStreamInfo)
}

func verifyExistingStreamRemoval(t *testing.T, existingFileStreams []DataStreamInfo, tempdir string, dirName string, restoredStreams []DataStreamInfo) {
	for _, currentFile := range existingFileStreams {
		fp := path.Join(tempdir, dirName, currentFile.name)

		existsInRestored := existsInStreamList(currentFile.name, restoredStreams)
		if !existsInRestored {
			//Stream that doesn't exist in the restored stream list must have been removed.
			_, err1 := os.Stat(fp)
			rtest.Assert(t, errors.Is(err1, os.ErrNotExist), "The file "+currentFile.name+" should not exist")
		}
	}

	for _, currentFile := range restoredStreams {
		fp := path.Join(tempdir, dirName, currentFile.name)
		verifyRestores(t, false, fp, currentFile)
	}
}

func existsInStreamList(name string, streams []DataStreamInfo) bool {
	for _, value := range streams {
		if value.name == name {
			return true
		}
	}
	return false
}

func TestAdsDirectory(t *testing.T) {
	streams := []DataStreamInfo{
		{"TestDirStream:datastream1:$DATA", "First dir stream."},
		{"TestDirStream:datastream2:$DATA", "Second dir stream."},
	}

	nodeinfo := NodeTestInfo{
		parentDir:          "dir",
		attributes:         &FileAttributes{},
		DataStreamInfo:     DataStreamInfo{name: "TestDirStream"},
		IsDirectory:        true,
		StreamRestoreOrder: []int{0, 1},
		AdsStreams:         streams,
	}

	tempDir := t.TempDir()
	runRestorerTest(t, nodeinfo, tempDir, false, NodeTestInfo{})

	for _, stream := range streams {
		fp := path.Join(tempDir, nodeinfo.parentDir, stream.name)
		verifyRestores(t, false, fp, stream)
	}

	dirPath := path.Join(tempDir, nodeinfo.parentDir, nodeinfo.name)
	verifyRestores(t, true, dirPath, nodeinfo.DataStreamInfo)
}
