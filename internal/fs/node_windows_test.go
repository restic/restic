//go:build windows
// +build windows

package fs

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/test"
	"golang.org/x/sys/windows"
)

func TestRestoreSecurityDescriptors(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
	for i, sd := range testFileSDs {
		testRestoreSecurityDescriptor(t, sd, tempDir, restic.NodeTypeFile, fmt.Sprintf("testfile%d", i))
	}
	for i, sd := range testDirSDs {
		testRestoreSecurityDescriptor(t, sd, tempDir, restic.NodeTypeDir, fmt.Sprintf("testdir%d", i))
	}
}

func testRestoreSecurityDescriptor(t *testing.T, sd string, tempDir string, fileType restic.NodeType, fileName string) {
	// Decode the encoded string SD to get the security descriptor input in bytes.
	sdInputBytes, err := base64.StdEncoding.DecodeString(sd)
	test.OK(t, errors.Wrapf(err, "Error decoding SD for: %s", fileName))
	// Wrap the security descriptor bytes in windows attributes and convert to generic attributes.
	genericAttributes, err := restic.WindowsAttrsToGenericAttributes(restic.WindowsAttributes{CreationTime: nil, FileAttributes: nil, SecurityDescriptor: &sdInputBytes})
	test.OK(t, errors.Wrapf(err, "Error constructing windows attributes for: %s", fileName))
	// Construct a Node with the generic attributes.
	expectedNode := getNode(fileName, fileType, genericAttributes)

	// Restore the file/dir and restore the meta data including the security descriptors.
	testPath, node := restoreAndGetNode(t, tempDir, &expectedNode, false)
	// Get the security descriptor from the node constructed from the file info of the restored path.
	sdByteFromRestoredNode := getWindowsAttr(t, testPath, node).SecurityDescriptor

	// Get the security descriptor for the test path after the restore.
	sdBytesFromRestoredPath, err := getSecurityDescriptor(testPath)
	test.OK(t, errors.Wrapf(err, "Error while getting the security descriptor for: %s", testPath))

	// Compare the input SD and the SD got from the restored file.
	compareSecurityDescriptors(t, testPath, sdInputBytes, *sdBytesFromRestoredPath)
	// Compare the SD got from node constructed from the restored file info and the SD got directly from the restored file.
	compareSecurityDescriptors(t, testPath, *sdByteFromRestoredNode, *sdBytesFromRestoredPath)
}

func getNode(name string, fileType restic.NodeType, genericAttributes map[restic.GenericAttributeType]json.RawMessage) restic.Node {
	return restic.Node{
		Name:              name,
		Type:              fileType,
		Mode:              0644,
		ModTime:           parseTime("2024-02-21 6:30:01.111"),
		AccessTime:        parseTime("2024-02-22 7:31:02.222"),
		ChangeTime:        parseTime("2024-02-23 8:32:03.333"),
		GenericAttributes: genericAttributes,
	}
}

func getWindowsAttr(t *testing.T, testPath string, node *restic.Node) restic.WindowsAttributes {
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
	attr := fi.Sys().(*syscall.Win32FileAttributeData)
	creationTimeAttribute := attr.CreationTime
	//Using the temp dir creation time as the test creation time for the test file and folder
	runGenericAttributesTest(t, path, restic.TypeCreationTime, restic.WindowsAttributes{CreationTime: &creationTimeAttribute}, false)
}

func TestRestoreFileAttributes(t *testing.T) {
	t.Parallel()
	genericAttributeName := restic.TypeFileAttributes
	tempDir := t.TempDir()
	normal := uint32(syscall.FILE_ATTRIBUTE_NORMAL)
	hidden := uint32(syscall.FILE_ATTRIBUTE_HIDDEN)
	system := uint32(syscall.FILE_ATTRIBUTE_SYSTEM)
	archive := uint32(syscall.FILE_ATTRIBUTE_ARCHIVE)
	encrypted := uint32(windows.FILE_ATTRIBUTE_ENCRYPTED)
	fileAttributes := []restic.WindowsAttributes{
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
		genericAttrs, err := restic.WindowsAttrsToGenericAttributes(fileAttr)
		test.OK(t, err)
		expectedNodes := []restic.Node{
			{
				Name:              fmt.Sprintf("testfile%d", i),
				Type:              restic.NodeTypeFile,
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
	folderAttributes := []restic.WindowsAttributes{
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
		genericAttrs, err := restic.WindowsAttrsToGenericAttributes(folderAttr)
		test.OK(t, err)
		expectedNodes := []restic.Node{
			{
				Name:              fmt.Sprintf("testdirectory%d", i),
				Type:              restic.NodeTypeDir,
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

func runGenericAttributesTest(t *testing.T, tempDir string, genericAttributeName restic.GenericAttributeType, genericAttributeExpected restic.WindowsAttributes, warningExpected bool) {
	genericAttributes, err := restic.WindowsAttrsToGenericAttributes(genericAttributeExpected)
	test.OK(t, err)
	expectedNodes := []restic.Node{
		{
			Name:              "testfile",
			Type:              restic.NodeTypeFile,
			Mode:              0644,
			ModTime:           parseTime("2005-05-14 21:07:03.111"),
			AccessTime:        parseTime("2005-05-14 21:07:04.222"),
			ChangeTime:        parseTime("2005-05-14 21:07:05.333"),
			GenericAttributes: genericAttributes,
		},
		{
			Name:              "testdirectory",
			Type:              restic.NodeTypeDir,
			Mode:              0755,
			ModTime:           parseTime("2005-05-14 21:07:03.111"),
			AccessTime:        parseTime("2005-05-14 21:07:04.222"),
			ChangeTime:        parseTime("2005-05-14 21:07:05.333"),
			GenericAttributes: genericAttributes,
		},
	}
	runGenericAttributesTestForNodes(t, expectedNodes, tempDir, genericAttributeName, genericAttributeExpected, warningExpected)
}
func runGenericAttributesTestForNodes(t *testing.T, expectedNodes []restic.Node, tempDir string, genericAttr restic.GenericAttributeType, genericAttributeExpected restic.WindowsAttributes, warningExpected bool) {

	for _, testNode := range expectedNodes {
		testPath, node := restoreAndGetNode(t, tempDir, &testNode, warningExpected)
		rawMessage := node.GenericAttributes[genericAttr]
		genericAttrsExpected, err := restic.WindowsAttrsToGenericAttributes(genericAttributeExpected)
		test.OK(t, err)
		rawMessageExpected := genericAttrsExpected[genericAttr]
		test.Equals(t, rawMessageExpected, rawMessage, "Generic attribute: %s got from NodeFromFileInfo not equal for path: %s", string(genericAttr), testPath)
	}
}

func restoreAndGetNode(t *testing.T, tempDir string, testNode *restic.Node, warningExpected bool) (string, *restic.Node) {
	testPath := filepath.Join(tempDir, "001", testNode.Name)
	err := os.MkdirAll(filepath.Dir(testPath), testNode.Mode)
	test.OK(t, errors.Wrapf(err, "Failed to create parent directories for: %s", testPath))

	if testNode.Type == restic.NodeTypeFile {

		testFile, err := os.Create(testPath)
		test.OK(t, errors.Wrapf(err, "Failed to create test file: %s", testPath))
		testFile.Close()
	} else if testNode.Type == restic.NodeTypeDir {

		err := os.Mkdir(testPath, testNode.Mode)
		test.OK(t, errors.Wrapf(err, "Failed to create test directory: %s", testPath))
	}

	err = NodeRestoreMetadata(testNode, testPath, func(msg string) {
		if warningExpected {
			test.Assert(t, warningExpected, "Warning triggered as expected: %s", msg)
		} else {
			// If warning is not expected, this code should not get triggered.
			test.OK(t, fmt.Errorf("Warning triggered for path: %s: %s", testPath, msg))
		}
	}, func(_ string) bool { return true })
	test.OK(t, errors.Wrapf(err, "Failed to restore metadata for: %s", testPath))

	fs := &Local{}
	meta, err := fs.OpenFile(testPath, O_NOFOLLOW, true)
	test.OK(t, err)
	nodeFromFileInfo, err := meta.ToNode(false)
	test.OK(t, errors.Wrapf(err, "Could not get NodeFromFileInfo for path: %s", testPath))
	test.OK(t, meta.Close())

	return testPath, nodeFromFileInfo
}

const TypeSomeNewAttribute restic.GenericAttributeType = "MockAttributes.SomeNewAttribute"

func TestNewGenericAttributeType(t *testing.T) {
	t.Parallel()

	newGenericAttribute := map[restic.GenericAttributeType]json.RawMessage{}
	newGenericAttribute[TypeSomeNewAttribute] = []byte("any value")

	tempDir := t.TempDir()
	expectedNodes := []restic.Node{
		{
			Name:              "testfile",
			Type:              restic.NodeTypeFile,
			Mode:              0644,
			ModTime:           parseTime("2005-05-14 21:07:03.111"),
			AccessTime:        parseTime("2005-05-14 21:07:04.222"),
			ChangeTime:        parseTime("2005-05-14 21:07:05.333"),
			GenericAttributes: newGenericAttribute,
		},
		{
			Name:              "testdirectory",
			Type:              restic.NodeTypeDir,
			Mode:              0755,
			ModTime:           parseTime("2005-05-14 21:07:03.111"),
			AccessTime:        parseTime("2005-05-14 21:07:04.222"),
			ChangeTime:        parseTime("2005-05-14 21:07:05.333"),
			GenericAttributes: newGenericAttribute,
		},
	}
	for _, testNode := range expectedNodes {
		testPath, node := restoreAndGetNode(t, tempDir, &testNode, true)
		_, ua, err := genericAttributesToWindowsAttrs(node.GenericAttributes)
		test.OK(t, err)
		// Since this GenericAttribute is unknown to this version of the software, it will not get set on the file.
		test.Assert(t, len(ua) == 0, "Unknown attributes: %s found for path: %s", ua, testPath)
	}
}

func TestRestoreExtendedAttributes(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
	expectedNodes := []restic.Node{
		{
			Name:       "testfile",
			Type:       restic.NodeTypeFile,
			Mode:       0644,
			ModTime:    parseTime("2005-05-14 21:07:03.111"),
			AccessTime: parseTime("2005-05-14 21:07:04.222"),
			ChangeTime: parseTime("2005-05-14 21:07:05.333"),
			ExtendedAttributes: []restic.ExtendedAttribute{
				{"user.foo", []byte("bar")},
			},
		},
		{
			Name:       "testdirectory",
			Type:       restic.NodeTypeDir,
			Mode:       0755,
			ModTime:    parseTime("2005-05-14 21:07:03.111"),
			AccessTime: parseTime("2005-05-14 21:07:04.222"),
			ChangeTime: parseTime("2005-05-14 21:07:05.333"),
			ExtendedAttributes: []restic.ExtendedAttribute{
				{"user.foo", []byte("bar")},
			},
		},
	}
	for _, testNode := range expectedNodes {
		testPath, node := restoreAndGetNode(t, tempDir, &testNode, false)

		var handle windows.Handle
		var err error
		utf16Path := windows.StringToUTF16Ptr(testPath)
		if node.Type == restic.NodeTypeFile {
			handle, err = windows.CreateFile(utf16Path, windows.FILE_READ_EA, 0, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
		} else if node.Type == restic.NodeTypeDir {
			handle, err = windows.CreateFile(utf16Path, windows.FILE_READ_EA, 0, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
		}
		test.OK(t, errors.Wrapf(err, "Error opening file/directory for: %s", testPath))
		defer func() {
			err := windows.Close(handle)
			test.OK(t, errors.Wrapf(err, "Error closing file for: %s", testPath))
		}()

		extAttr, err := fgetEA(handle)
		test.OK(t, errors.Wrapf(err, "Error getting extended attributes for: %s", testPath))
		test.Equals(t, len(node.ExtendedAttributes), len(extAttr))

		for _, expectedExtAttr := range node.ExtendedAttributes {
			var foundExtAttr *extendedAttribute
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

func TestPrepareVolumeName(t *testing.T) {
	currentVolume := filepath.VolumeName(func() string {
		// Get the current working directory
		pwd, err := os.Getwd()
		if err != nil {
			t.Fatalf("Failed to get current working directory: %v", err)
		}
		return pwd
	}())
	// Create a temporary directory for the test
	tempDir, err := os.MkdirTemp("", "restic_test_"+time.Now().Format("20060102150405"))
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a long file name
	longFileName := `\Very\Long\Path\That\Exceeds\260\Characters\` + strings.Repeat(`\VeryLongFolderName`, 20) + `\\LongFile.txt`
	longFilePath := filepath.Join(tempDir, longFileName)

	tempDirVolume := filepath.VolumeName(tempDir)
	// Create the file
	content := []byte("This is a test file with a very long name.")
	err = os.MkdirAll(filepath.Dir(longFilePath), 0755)
	test.OK(t, err)
	if err != nil {
		t.Fatalf("Failed to create long folder: %v", err)
	}
	err = os.WriteFile(longFilePath, content, 0644)
	test.OK(t, err)
	if err != nil {
		t.Fatalf("Failed to create long file: %v", err)
	}
	osVolumeGUIDPath := getOSVolumeGUIDPath(t)
	osVolumeGUIDVolume := filepath.VolumeName(osVolumeGUIDPath)

	testCases := []struct {
		name                string
		path                string
		expectedVolume      string
		expectError         bool
		expectedEASupported bool
		isRealPath          bool
	}{
		{
			name:                "Network drive path",
			path:                `Z:\Shared\Documents`,
			expectedVolume:      `Z:`,
			expectError:         false,
			expectedEASupported: false,
		},
		{
			name:                "Subst drive path",
			path:                `X:\Virtual\Folder`,
			expectedVolume:      `X:`,
			expectError:         false,
			expectedEASupported: false,
		},
		{
			name:                "Windows reserved path",
			path:                `\\.\` + os.Getenv("SystemDrive") + `\System32\drivers\etc\hosts`,
			expectedVolume:      `\\.\` + os.Getenv("SystemDrive"),
			expectError:         false,
			expectedEASupported: true,
			isRealPath:          true,
		},
		{
			name:                "Long UNC path",
			path:                `\\?\UNC\LongServerName\VeryLongShareName\DeepPath\File.txt`,
			expectedVolume:      `\\LongServerName\VeryLongShareName`,
			expectError:         false,
			expectedEASupported: false,
		},
		{
			name:                "Volume GUID path",
			path:                osVolumeGUIDPath,
			expectedVolume:      osVolumeGUIDVolume,
			expectError:         false,
			expectedEASupported: true,
			isRealPath:          true,
		},
		{
			name:                "Volume GUID path with subfolder",
			path:                osVolumeGUIDPath + `\Windows`,
			expectedVolume:      osVolumeGUIDVolume,
			expectError:         false,
			expectedEASupported: true,
			isRealPath:          true,
		},
		{
			name:                "Standard path",
			path:                os.Getenv("SystemDrive") + `\Users\`,
			expectedVolume:      os.Getenv("SystemDrive"),
			expectError:         false,
			expectedEASupported: true,
			isRealPath:          true,
		},
		{
			name:                "Extended length path",
			path:                longFilePath,
			expectedVolume:      tempDirVolume,
			expectError:         false,
			expectedEASupported: true,
			isRealPath:          true,
		},
		{
			name:                "UNC path",
			path:                `\\server\share\folder`,
			expectedVolume:      `\\server\share`,
			expectError:         false,
			expectedEASupported: false,
		},
		{
			name:                "Extended UNC path",
			path:                `\\?\UNC\server\share\folder`,
			expectedVolume:      `\\server\share`,
			expectError:         false,
			expectedEASupported: false,
		},
		{
			name:                "Volume Shadow Copy root",
			path:                `\\?\GLOBALROOT\Device\HarddiskVolumeShadowCopy5555`,
			expectedVolume:      `\\?\GLOBALROOT\Device\HarddiskVolumeShadowCopy5555`,
			expectError:         false,
			expectedEASupported: false,
		},
		{
			name:                "Volume Shadow Copy path",
			path:                `\\?\GLOBALROOT\Device\HarddiskVolumeShadowCopy5555\Users\test`,
			expectedVolume:      `\\?\GLOBALROOT\Device\HarddiskVolumeShadowCopy5555`,
			expectError:         false,
			expectedEASupported: false,
		},
		{
			name: "Relative path",
			path: `folder\subfolder`,

			expectedVolume:      currentVolume, // Get current volume
			expectError:         false,
			expectedEASupported: true,
		},
		{
			name:                "Empty path",
			path:                ``,
			expectedVolume:      currentVolume,
			expectError:         false,
			expectedEASupported: true,
			isRealPath:          false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			isEASupported, err := checkAndStoreEASupport(tc.path)
			test.OK(t, err)
			test.Equals(t, tc.expectedEASupported, isEASupported)

			volume, err := prepareVolumeName(tc.path)

			if tc.expectError {
				test.Assert(t, err != nil, "Expected an error, but got none")
			} else {
				test.OK(t, err)
			}
			test.Equals(t, tc.expectedVolume, volume)

			if tc.isRealPath {
				isEASupportedVolume, err := pathSupportsExtendedAttributes(volume + `\`)
				// If the prepared volume name is not valid, we will next fetch the actual volume name.
				test.OK(t, err)

				test.Equals(t, tc.expectedEASupported, isEASupportedVolume)

				actualVolume, err := getVolumePathName(tc.path)
				test.OK(t, err)
				test.Equals(t, tc.expectedVolume, actualVolume)
			}
		})
	}
}

func getOSVolumeGUIDPath(t *testing.T) string {
	// Get the path of the OS drive (usually C:\)
	osDrive := os.Getenv("SystemDrive") + "\\"

	// Convert to a volume GUID path
	volumeName, err := windows.UTF16PtrFromString(osDrive)
	test.OK(t, err)
	if err != nil {
		return ""
	}

	var volumeGUID [windows.MAX_PATH]uint16
	err = windows.GetVolumeNameForVolumeMountPoint(volumeName, &volumeGUID[0], windows.MAX_PATH)
	test.OK(t, err)
	if err != nil {
		return ""
	}

	return windows.UTF16ToString(volumeGUID[:])
}

func TestGetVolumePathName(t *testing.T) {
	tempDirVolume := filepath.VolumeName(os.TempDir())
	testCases := []struct {
		name           string
		path           string
		expectedPrefix string
	}{
		{
			name:           "Root directory",
			path:           os.Getenv("SystemDrive") + `\`,
			expectedPrefix: os.Getenv("SystemDrive"),
		},
		{
			name:           "Nested directory",
			path:           os.Getenv("SystemDrive") + `\Windows\System32`,
			expectedPrefix: os.Getenv("SystemDrive"),
		},
		{
			name:           "Temp directory",
			path:           os.TempDir() + `\`,
			expectedPrefix: tempDirVolume,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			volumeName, err := getVolumePathName(tc.path)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
			if !strings.HasPrefix(volumeName, tc.expectedPrefix) {
				t.Errorf("Expected volume name to start with %s, but got %s", tc.expectedPrefix, volumeName)
			}
		})
	}

	// Test with an invalid path
	_, err := getVolumePathName("Z:\\NonExistentPath")
	if err == nil {
		t.Error("Expected an error for non-existent path, but got nil")
	}
}
