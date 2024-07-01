//go:build windows
// +build windows

package restic

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/fs"
	"github.com/restic/restic/internal/test"
	"golang.org/x/sys/windows"
)

func TestRestoreSecurityDescriptors(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
	for i, sd := range fs.TestFileSDs {
		testRestoreSecurityDescriptor(t, sd, tempDir, "file", fmt.Sprintf("testfile%d", i))
	}
	for i, sd := range fs.TestDirSDs {
		testRestoreSecurityDescriptor(t, sd, tempDir, "dir", fmt.Sprintf("testdir%d", i))
	}
}

func testRestoreSecurityDescriptor(t *testing.T, sd string, tempDir, fileType, fileName string) {
	// Decode the encoded string SD to get the security descriptor input in bytes.
	sdInputBytes, err := base64.StdEncoding.DecodeString(sd)
	test.OK(t, errors.Wrapf(err, "Error decoding SD for: %s", fileName))
	// Wrap the security descriptor bytes in windows attributes and convert to generic attributes.
	genericAttributes, err := WindowsAttrsToGenericAttributes(WindowsAttributes{CreationTime: nil, FileAttributes: nil, SecurityDescriptor: &sdInputBytes})
	test.OK(t, errors.Wrapf(err, "Error constructing windows attributes for: %s", fileName))
	// Construct a Node with the generic attributes.
	expectedNode := getNode(fileName, fileType, genericAttributes)

	// Restore the file/dir and restore the meta data including the security descriptors.
	testPath, node := restoreAndGetNode(t, tempDir, expectedNode, false)
	// Get the security descriptor from the node constructed from the file info of the restored path.
	sdByteFromRestoredNode := getWindowsAttr(t, testPath, node).SecurityDescriptor

	// Get the security descriptor for the test path after the restore.
	sdBytesFromRestoredPath, err := fs.GetSecurityDescriptor(testPath)
	test.OK(t, errors.Wrapf(err, "Error while getting the security descriptor for: %s", testPath))

	// Compare the input SD and the SD got from the restored file.
	fs.CompareSecurityDescriptors(t, testPath, sdInputBytes, *sdBytesFromRestoredPath)
	// Compare the SD got from node constructed from the restored file info and the SD got directly from the restored file.
	fs.CompareSecurityDescriptors(t, testPath, *sdByteFromRestoredNode, *sdBytesFromRestoredPath)
}

func getNode(name string, fileType string, genericAttributes map[GenericAttributeType]json.RawMessage) Node {
	return Node{
		Name:              name,
		Type:              fileType,
		Mode:              0644,
		ModTime:           parseTime("2024-02-21 6:30:01.111"),
		AccessTime:        parseTime("2024-02-22 7:31:02.222"),
		ChangeTime:        parseTime("2024-02-23 8:32:03.333"),
		GenericAttributes: genericAttributes,
	}
}

func getWindowsAttr(t *testing.T, testPath string, node *Node) WindowsAttributes {
	windowsAttributes, unknownAttribs, err := genericAttributesToWindowsAttrs(node.GenericAttributes)
	test.OK(t, errors.Wrapf(err, "Error getting windows attr from generic attr: %s", testPath))
	test.Assert(t, len(unknownAttribs) == 0, "Unknown attribs found: %s for: %s", unknownAttribs, testPath)
	return windowsAttributes
}

func TestRestoreCreationTime(t *testing.T) {
	t.Parallel()
	path := t.TempDir()
	fi, err := os.Lstat(path)
	test.OK(t, errors.Wrapf(err, "Could not Lstat for path: %s", path))
	creationTimeAttribute := getCreationTime(fi, path)
	test.OK(t, errors.Wrapf(err, "Could not get creation time for path: %s", path))
	//Using the temp dir creation time as the test creation time for the test file and folder
	runGenericAttributesTest(t, path, TypeCreationTime, WindowsAttributes{CreationTime: creationTimeAttribute}, false)
}

func TestRestoreFileAttributes(t *testing.T) {
	t.Parallel()
	genericAttributeName := TypeFileAttributes
	tempDir := t.TempDir()
	normal := uint32(syscall.FILE_ATTRIBUTE_NORMAL)
	hidden := uint32(syscall.FILE_ATTRIBUTE_HIDDEN)
	system := uint32(syscall.FILE_ATTRIBUTE_SYSTEM)
	archive := uint32(syscall.FILE_ATTRIBUTE_ARCHIVE)
	encrypted := uint32(windows.FILE_ATTRIBUTE_ENCRYPTED)
	fileAttributes := []WindowsAttributes{
		//normal
		{FileAttributes: &normal},
		//hidden
		{FileAttributes: &hidden},
		//system
		{FileAttributes: &system},
		//archive
		{FileAttributes: &archive},
		//encrypted
		{FileAttributes: &encrypted},
	}
	for i, fileAttr := range fileAttributes {
		genericAttrs, err := WindowsAttrsToGenericAttributes(fileAttr)
		test.OK(t, err)
		expectedNodes := []Node{
			{
				Name:              fmt.Sprintf("testfile%d", i),
				Type:              "file",
				Mode:              0655,
				ModTime:           parseTime("2005-05-14 21:07:03.111"),
				AccessTime:        parseTime("2005-05-14 21:07:04.222"),
				ChangeTime:        parseTime("2005-05-14 21:07:05.333"),
				GenericAttributes: genericAttrs,
			},
		}
		runGenericAttributesTestForNodes(t, expectedNodes, tempDir, genericAttributeName, fileAttr, false)
	}
	normal = uint32(syscall.FILE_ATTRIBUTE_DIRECTORY)
	hidden = uint32(syscall.FILE_ATTRIBUTE_DIRECTORY | syscall.FILE_ATTRIBUTE_HIDDEN)
	system = uint32(syscall.FILE_ATTRIBUTE_DIRECTORY | windows.FILE_ATTRIBUTE_SYSTEM)
	archive = uint32(syscall.FILE_ATTRIBUTE_DIRECTORY | windows.FILE_ATTRIBUTE_ARCHIVE)
	encrypted = uint32(syscall.FILE_ATTRIBUTE_DIRECTORY | windows.FILE_ATTRIBUTE_ENCRYPTED)
	folderAttributes := []WindowsAttributes{
		//normal
		{FileAttributes: &normal},
		//hidden
		{FileAttributes: &hidden},
		//system
		{FileAttributes: &system},
		//archive
		{FileAttributes: &archive},
		//encrypted
		{FileAttributes: &encrypted},
	}
	for i, folderAttr := range folderAttributes {
		genericAttrs, err := WindowsAttrsToGenericAttributes(folderAttr)
		test.OK(t, err)
		expectedNodes := []Node{
			{
				Name:              fmt.Sprintf("testdirectory%d", i),
				Type:              "dir",
				Mode:              0755,
				ModTime:           parseTime("2005-05-14 21:07:03.111"),
				AccessTime:        parseTime("2005-05-14 21:07:04.222"),
				ChangeTime:        parseTime("2005-05-14 21:07:05.333"),
				GenericAttributes: genericAttrs,
			},
		}
		runGenericAttributesTestForNodes(t, expectedNodes, tempDir, genericAttributeName, folderAttr, false)
	}
}

func runGenericAttributesTest(t *testing.T, tempDir string, genericAttributeName GenericAttributeType, genericAttributeExpected WindowsAttributes, warningExpected bool) {
	genericAttributes, err := WindowsAttrsToGenericAttributes(genericAttributeExpected)
	test.OK(t, err)
	expectedNodes := []Node{
		{
			Name:              "testfile",
			Type:              "file",
			Mode:              0644,
			ModTime:           parseTime("2005-05-14 21:07:03.111"),
			AccessTime:        parseTime("2005-05-14 21:07:04.222"),
			ChangeTime:        parseTime("2005-05-14 21:07:05.333"),
			GenericAttributes: genericAttributes,
		},
		{
			Name:              "testdirectory",
			Type:              "dir",
			Mode:              0755,
			ModTime:           parseTime("2005-05-14 21:07:03.111"),
			AccessTime:        parseTime("2005-05-14 21:07:04.222"),
			ChangeTime:        parseTime("2005-05-14 21:07:05.333"),
			GenericAttributes: genericAttributes,
		},
	}
	runGenericAttributesTestForNodes(t, expectedNodes, tempDir, genericAttributeName, genericAttributeExpected, warningExpected)
}
func runGenericAttributesTestForNodes(t *testing.T, expectedNodes []Node, tempDir string, genericAttr GenericAttributeType, genericAttributeExpected WindowsAttributes, warningExpected bool) {

	for _, testNode := range expectedNodes {
		testPath, node := restoreAndGetNode(t, tempDir, testNode, warningExpected)
		rawMessage := node.GenericAttributes[genericAttr]
		genericAttrsExpected, err := WindowsAttrsToGenericAttributes(genericAttributeExpected)
		test.OK(t, err)
		rawMessageExpected := genericAttrsExpected[genericAttr]
		test.Equals(t, rawMessageExpected, rawMessage, "Generic attribute: %s got from NodeFromFileInfo not equal for path: %s", string(genericAttr), testPath)
	}
}

func restoreAndGetNode(t *testing.T, tempDir string, testNode Node, warningExpected bool) (string, *Node) {
	testPath := filepath.Join(tempDir, "001", testNode.Name)
	err := os.MkdirAll(filepath.Dir(testPath), testNode.Mode)
	test.OK(t, errors.Wrapf(err, "Failed to create parent directories for: %s", testPath))

	if testNode.Type == "file" {

		testFile, err := os.Create(testPath)
		test.OK(t, errors.Wrapf(err, "Failed to create test file: %s", testPath))
		testFile.Close()
	} else if testNode.Type == "dir" {

		err := os.Mkdir(testPath, testNode.Mode)
		test.OK(t, errors.Wrapf(err, "Failed to create test directory: %s", testPath))
	}

	err = testNode.RestoreMetadata(testPath, func(msg string) {
		if warningExpected {
			test.Assert(t, warningExpected, "Warning triggered as expected: %s", msg)
		} else {
			// If warning is not expected, this code should not get triggered.
			test.OK(t, fmt.Errorf("Warning triggered for path: %s: %s", testPath, msg))
		}
	})
	test.OK(t, errors.Wrapf(err, "Failed to restore metadata for: %s", testPath))

	fi, err := os.Lstat(testPath)
	test.OK(t, errors.Wrapf(err, "Could not Lstat for path: %s", testPath))

	nodeFromFileInfo, err := NodeFromFileInfo(testPath, fi, false)
	test.OK(t, errors.Wrapf(err, "Could not get NodeFromFileInfo for path: %s", testPath))

	return testPath, nodeFromFileInfo
}

const TypeSomeNewAttribute GenericAttributeType = "MockAttributes.SomeNewAttribute"

func TestNewGenericAttributeType(t *testing.T) {
	t.Parallel()

	newGenericAttribute := map[GenericAttributeType]json.RawMessage{}
	newGenericAttribute[TypeSomeNewAttribute] = []byte("any value")

	tempDir := t.TempDir()
	expectedNodes := []Node{
		{
			Name:              "testfile",
			Type:              "file",
			Mode:              0644,
			ModTime:           parseTime("2005-05-14 21:07:03.111"),
			AccessTime:        parseTime("2005-05-14 21:07:04.222"),
			ChangeTime:        parseTime("2005-05-14 21:07:05.333"),
			GenericAttributes: newGenericAttribute,
		},
		{
			Name:              "testdirectory",
			Type:              "dir",
			Mode:              0755,
			ModTime:           parseTime("2005-05-14 21:07:03.111"),
			AccessTime:        parseTime("2005-05-14 21:07:04.222"),
			ChangeTime:        parseTime("2005-05-14 21:07:05.333"),
			GenericAttributes: newGenericAttribute,
		},
	}
	for _, testNode := range expectedNodes {
		testPath, node := restoreAndGetNode(t, tempDir, testNode, true)
		_, ua, err := genericAttributesToWindowsAttrs(node.GenericAttributes)
		test.OK(t, err)
		// Since this GenericAttribute is unknown to this version of the software, it will not get set on the file.
		test.Assert(t, len(ua) == 0, "Unknown attributes: %s found for path: %s", ua, testPath)
	}
}

func TestRestoreExtendedAttributes(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
	expectedNodes := []Node{
		{
			Name:       "testfile",
			Type:       "file",
			Mode:       0644,
			ModTime:    parseTime("2005-05-14 21:07:03.111"),
			AccessTime: parseTime("2005-05-14 21:07:04.222"),
			ChangeTime: parseTime("2005-05-14 21:07:05.333"),
			ExtendedAttributes: []ExtendedAttribute{
				{"user.foo", []byte("bar")},
			},
		},
		{
			Name:       "testdirectory",
			Type:       "dir",
			Mode:       0755,
			ModTime:    parseTime("2005-05-14 21:07:03.111"),
			AccessTime: parseTime("2005-05-14 21:07:04.222"),
			ChangeTime: parseTime("2005-05-14 21:07:05.333"),
			ExtendedAttributes: []ExtendedAttribute{
				{"user.foo", []byte("bar")},
			},
		},
	}
	for _, testNode := range expectedNodes {
		testPath, node := restoreAndGetNode(t, tempDir, testNode, false)

		var handle windows.Handle
		var err error
		utf16Path := windows.StringToUTF16Ptr(testPath)
		if node.Type == "file" {
			handle, err = windows.CreateFile(utf16Path, windows.FILE_READ_EA, 0, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
		} else if node.Type == "dir" {
			handle, err = windows.CreateFile(utf16Path, windows.FILE_READ_EA, 0, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
		}
		test.OK(t, errors.Wrapf(err, "Error opening file/directory for: %s", testPath))
		defer func() {
			err := windows.Close(handle)
			test.OK(t, errors.Wrapf(err, "Error closing file for: %s", testPath))
		}()

		extAttr, err := fs.GetFileEA(handle)
		test.OK(t, errors.Wrapf(err, "Error getting extended attributes for: %s", testPath))
		test.Equals(t, len(node.ExtendedAttributes), len(extAttr))

		for _, expectedExtAttr := range node.ExtendedAttributes {
			var foundExtAttr *fs.ExtendedAttribute
			for _, ea := range extAttr {
				if strings.EqualFold(ea.Name, expectedExtAttr.Name) {
					foundExtAttr = &ea
					break

				}
			}
			test.Assert(t, foundExtAttr != nil, "Expected extended attribute not found")
			test.Equals(t, expectedExtAttr.Value, foundExtAttr.Value)
		}
	}
}
