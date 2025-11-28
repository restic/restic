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

	"github.com/restic/restic/internal/data"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/test"
	"golang.org/x/sys/windows"
)

func TestRestoreSecurityDescriptors(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
	for i, sd := range testFileSDs {
		testRestoreSecurityDescriptor(t, sd, tempDir, data.NodeTypeFile, fmt.Sprintf("testfile%d", i))
	}
	for i, sd := range testDirSDs {
		testRestoreSecurityDescriptor(t, sd, tempDir, data.NodeTypeDir, fmt.Sprintf("testdir%d", i))
	}
}

func testRestoreSecurityDescriptor(t *testing.T, sd string, tempDir string, fileType data.NodeType, fileName string) {
	// Decode the encoded string SD to get the security descriptor input in bytes.
	sdInputBytes, err := base64.StdEncoding.DecodeString(sd)
	test.OK(t, errors.Wrapf(err, "Error decoding SD for: %s", fileName))
	// Wrap the security descriptor bytes in windows attributes and convert to generic attributes.
	genericAttributes, err := data.WindowsAttrsToGenericAttributes(data.WindowsAttributes{CreationTime: nil, FileAttributes: nil, SecurityDescriptor: &sdInputBytes})
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

// TestRestoreSecurityDescriptorInheritance checks that the DACL protection (inheritance)
// flags are correctly restored. This is the mechanism that preserves the `IsInherited`
// property on individual ACEs.
func TestRestoreSecurityDescriptorInheritance(t *testing.T) {
	// This test requires admin privileges to manipulate ACLs effectively.
	isAdmin, err := isAdmin()
	test.OK(t, err)
	if !isAdmin {
		t.Skip("Skipping inheritance test, requires admin privileges")
	}

	tempDir := t.TempDir()

	// 1. Create a parent/child directory structure.
	parentDir := filepath.Join(tempDir, "parent")
	err = os.Mkdir(parentDir, 0755)
	test.OK(t, err)

	childDir := filepath.Join(parentDir, "child")
	err = os.Mkdir(childDir, 0755)
	test.OK(t, err)

	// 2. Set inheritable permissions on the parent.
	// We will give the "Users" group inheritable read access.
	users, err := windows.StringToSid("S-1-5-32-545") // BUILTIN\Users
	test.OK(t, err)

	// Create an EXPLICIT_ACCESS structure for the new ACE.
	explicitAccess := windows.EXPLICIT_ACCESS{
		AccessPermissions: windows.GENERIC_READ,
		AccessMode:        windows.GRANT_ACCESS,
		Inheritance:       windows.OBJECT_INHERIT_ACE | windows.CONTAINER_INHERIT_ACE,
		Trustee: windows.TRUSTEE{
			TrusteeForm:  windows.TRUSTEE_IS_SID,
			TrusteeType:  windows.TRUSTEE_IS_GROUP,
			TrusteeValue: windows.TrusteeValueFromSID(users),
		},
	}

	// Create a new DACL from the entry.
	dacl, err := windows.ACLFromEntries([]windows.EXPLICIT_ACCESS{explicitAccess}, nil)
	test.OK(t, err)

	// Apply this new DACL to the parent, marking it as unprotected so it can be inherited.
	err = windows.SetNamedSecurityInfo(
		parentDir,
		windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.UNPROTECTED_DACL_SECURITY_INFORMATION,
		nil, nil, dacl, nil,
	)
	test.OK(t, errors.Wrapf(err, "failed to set inheritable ACL on parent dir"))

	// 3. Get the Security Descriptor of the child, which should now have inherited the ACE.
	sdBytesOriginal, err := getSecurityDescriptor(childDir)
	test.OK(t, err)

	// Sanity check: verify the original child SD is NOT protected from inheritance.
	sdOriginal, err := securityDescriptorBytesToStruct(*sdBytesOriginal)
	test.OK(t, err)
	control, _, err := sdOriginal.Control()
	test.OK(t, err)
	test.Assert(t, control&windows.SE_DACL_PROTECTED == 0, "Pre-condition failed: child directory should have inheritance enabled")

	// 4. Create a restic node for the child directory.
	genericAttrs, err := data.WindowsAttrsToGenericAttributes(data.WindowsAttributes{SecurityDescriptor: sdBytesOriginal})
	test.OK(t, err)
	childNode := getNode("child-restored", "dir", genericAttrs)

	// 5. Restore the node to a new location.
	restoreDir := filepath.Join(tempDir, "restore")
	err = os.Mkdir(restoreDir, 0755)
	test.OK(t, err)

	restoredPath, _ := restoreAndGetNode(t, restoreDir, &childNode, false)

	// 6. Get the Security Descriptor of the restored child directory.
	sdBytesRestored, err := getSecurityDescriptor(restoredPath)
	test.OK(t, err)

	// 7. Compare the control flags of the original and restored SDs.
	sdRestored, err := securityDescriptorBytesToStruct(*sdBytesRestored)
	test.OK(t, err)
	controlRestored, _, err := sdRestored.Control()
	test.OK(t, err)

	// The core of the test: Ensure the restored DACL protection flag matches the original.
	originalIsProtected := (control & windows.SE_DACL_PROTECTED) != 0
	restoredIsProtected := (controlRestored & windows.SE_DACL_PROTECTED) != 0
	test.Equals(t, originalIsProtected, restoredIsProtected, "DACL protection flag was not restored correctly. Inheritance state is wrong.")
}

// TestRestoreSecurityDescriptorInheritanceLowPrivilege tests that the low-privilege restore
// path (setNamedSecurityInfoLow) correctly handles inheritance flags. This test doesn't require
// admin privileges and focuses on DACL restoration only.
func TestRestoreSecurityDescriptorInheritanceLowPrivilege(t *testing.T) {
	tempDir := t.TempDir()

	// 1. Create a test directory
	testDir := filepath.Join(tempDir, "testdir")
	err := os.Mkdir(testDir, 0755)
	test.OK(t, err)

	// 2. Get its security descriptor (which will have some default ACL)
	sdBytesOriginal, err := getSecurityDescriptor(testDir)
	test.OK(t, err)

	// Verify we can get the control flags
	sdOriginal, err := securityDescriptorBytesToStruct(*sdBytesOriginal)
	test.OK(t, err)
	controlOriginal, _, err := sdOriginal.Control()
	test.OK(t, err)

	// 3. Test both protected and unprotected scenarios by modifying the control flags
	testCases := []struct {
		name              string
		shouldBeProtected bool
	}{
		{"unprotected_inheritance_enabled", false},
		{"protected_inheritance_disabled", true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create a copy of the SD bytes to modify
			sdBytesTest := make([]byte, len(*sdBytesOriginal))
			copy(sdBytesTest, *sdBytesOriginal)

			// Modify the control flags to simulate protected/unprotected DACL
			sdTest, err := securityDescriptorBytesToStruct(sdBytesTest)
			test.OK(t, err)

			// Get the DACL from the test SD
			dacl, _, err := sdTest.DACL()
			test.OK(t, err)

			// Determine which control flag to use based on test case
			var controlToUse windows.SECURITY_DESCRIPTOR_CONTROL
			if tc.shouldBeProtected {
				controlToUse = controlOriginal | windows.SE_DACL_PROTECTED
			} else {
				controlToUse = controlOriginal &^ windows.SE_DACL_PROTECTED
			}

			// 4. Call setNamedSecurityInfoLow directly to test the low-privilege path
			restoreTarget := filepath.Join(tempDir, "restore_"+tc.name)
			err = os.Mkdir(restoreTarget, 0755)
			test.OK(t, err)

			// This directly tests the low-privilege restore function
			err = setNamedSecurityInfoLow(restoreTarget, dacl, controlToUse)
			test.OK(t, err)

			// 5. Get the security descriptor of the restored directory
			sdBytesRestored, err := getSecurityDescriptor(restoreTarget)
			test.OK(t, err)

			// 6. Verify that the control flags were correctly restored
			sdRestored, err := securityDescriptorBytesToStruct(*sdBytesRestored)
			test.OK(t, err)
			controlRestored, _, err := sdRestored.Control()
			test.OK(t, err) // Check if the protection flag matches what we requested
			restoredIsProtected := (controlRestored & windows.SE_DACL_PROTECTED) != 0
			if tc.shouldBeProtected != restoredIsProtected {
				t.Errorf("DACL protection flag was not restored correctly in low-privilege path. Expected protected=%v, got protected=%v",
					tc.shouldBeProtected, restoredIsProtected)
			}
		})
	}
}

func getNode(name string, fileType data.NodeType, genericAttributes map[data.GenericAttributeType]json.RawMessage) data.Node {
	return data.Node{
		Name:              name,
		Type:              fileType,
		Mode:              0644,
		ModTime:           parseTime("2024-02-21 6:30:01.111"),
		AccessTime:        parseTime("2024-02-22 7:31:02.222"),
		ChangeTime:        parseTime("2024-02-23 8:32:03.333"),
		GenericAttributes: genericAttributes,
	}
}

func getWindowsAttr(t *testing.T, testPath string, node *data.Node) data.WindowsAttributes {
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
	runGenericAttributesTest(t, path, data.TypeCreationTime, data.WindowsAttributes{CreationTime: &creationTimeAttribute}, false)
}

func TestRestoreFileAttributes(t *testing.T) {
	t.Parallel()
	genericAttributeName := data.TypeFileAttributes
	tempDir := t.TempDir()
	normal := uint32(syscall.FILE_ATTRIBUTE_NORMAL)
	hidden := uint32(syscall.FILE_ATTRIBUTE_HIDDEN)
	system := uint32(syscall.FILE_ATTRIBUTE_SYSTEM)
	archive := uint32(syscall.FILE_ATTRIBUTE_ARCHIVE)
	encrypted := uint32(windows.FILE_ATTRIBUTE_ENCRYPTED)
	fileAttributes := []data.WindowsAttributes{
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
		genericAttrs, err := data.WindowsAttrsToGenericAttributes(fileAttr)
		test.OK(t, err)
		expectedNodes := []data.Node{
			{
				Name:              fmt.Sprintf("testfile%d", i),
				Type:              data.NodeTypeFile,
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
	folderAttributes := []data.WindowsAttributes{
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
		genericAttrs, err := data.WindowsAttrsToGenericAttributes(folderAttr)
		test.OK(t, err)
		expectedNodes := []data.Node{
			{
				Name:              fmt.Sprintf("testdirectory%d", i),
				Type:              data.NodeTypeDir,
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

func runGenericAttributesTest(t *testing.T, tempDir string, genericAttributeName data.GenericAttributeType, genericAttributeExpected data.WindowsAttributes, warningExpected bool) {
	genericAttributes, err := data.WindowsAttrsToGenericAttributes(genericAttributeExpected)
	test.OK(t, err)
	expectedNodes := []data.Node{
		{
			Name:              "testfile",
			Type:              data.NodeTypeFile,
			Mode:              0644,
			ModTime:           parseTime("2005-05-14 21:07:03.111"),
			AccessTime:        parseTime("2005-05-14 21:07:04.222"),
			ChangeTime:        parseTime("2005-05-14 21:07:05.333"),
			GenericAttributes: genericAttributes,
		},
		{
			Name:              "testdirectory",
			Type:              data.NodeTypeDir,
			Mode:              0755,
			ModTime:           parseTime("2005-05-14 21:07:03.111"),
			AccessTime:        parseTime("2005-05-14 21:07:04.222"),
			ChangeTime:        parseTime("2005-05-14 21:07:05.333"),
			GenericAttributes: genericAttributes,
		},
	}
	runGenericAttributesTestForNodes(t, expectedNodes, tempDir, genericAttributeName, genericAttributeExpected, warningExpected)
}
func runGenericAttributesTestForNodes(t *testing.T, expectedNodes []data.Node, tempDir string, genericAttr data.GenericAttributeType, genericAttributeExpected data.WindowsAttributes, warningExpected bool) {

	for _, testNode := range expectedNodes {
		testPath, node := restoreAndGetNode(t, tempDir, &testNode, warningExpected)
		rawMessage := node.GenericAttributes[genericAttr]
		genericAttrsExpected, err := data.WindowsAttrsToGenericAttributes(genericAttributeExpected)
		test.OK(t, err)
		rawMessageExpected := genericAttrsExpected[genericAttr]
		test.Equals(t, rawMessageExpected, rawMessage, "Generic attribute: %s got from NodeFromFileInfo not equal for path: %s", string(genericAttr), testPath)
	}
}

func restoreAndGetNode(t *testing.T, tempDir string, testNode *data.Node, warningExpected bool) (string, *data.Node) {
	testPath := filepath.Join(tempDir, "001", testNode.Name)
	err := os.MkdirAll(filepath.Dir(testPath), testNode.Mode)
	test.OK(t, errors.Wrapf(err, "Failed to create parent directories for: %s", testPath))

	if testNode.Type == data.NodeTypeFile {

		testFile, err := os.Create(testPath)
		test.OK(t, errors.Wrapf(err, "Failed to create test file: %s", testPath))
		testFile.Close()
	} else if testNode.Type == data.NodeTypeDir {

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
	}, func(_ string) bool { return true }, false)
	test.OK(t, errors.Wrapf(err, "Failed to restore metadata for: %s", testPath))

	fs := &Local{}
	meta, err := fs.OpenFile(testPath, O_NOFOLLOW, true)
	test.OK(t, err)
	nodeFromFileInfo, err := meta.ToNode(false, t.Logf)
	test.OK(t, errors.Wrapf(err, "Could not get NodeFromFileInfo for path: %s", testPath))
	test.OK(t, meta.Close())

	return testPath, nodeFromFileInfo
}

const TypeSomeNewAttribute data.GenericAttributeType = "MockAttributes.SomeNewAttribute"

func TestNewGenericAttributeType(t *testing.T) {
	t.Parallel()

	newGenericAttribute := map[data.GenericAttributeType]json.RawMessage{}
	newGenericAttribute[TypeSomeNewAttribute] = []byte("any value")

	tempDir := t.TempDir()
	expectedNodes := []data.Node{
		{
			Name:              "testfile",
			Type:              data.NodeTypeFile,
			Mode:              0644,
			ModTime:           parseTime("2005-05-14 21:07:03.111"),
			AccessTime:        parseTime("2005-05-14 21:07:04.222"),
			ChangeTime:        parseTime("2005-05-14 21:07:05.333"),
			GenericAttributes: newGenericAttribute,
		},
		{
			Name:              "testdirectory",
			Type:              data.NodeTypeDir,
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
	expectedNodes := []data.Node{
		{
			Name:       "testfile",
			Type:       data.NodeTypeFile,
			Mode:       0644,
			ModTime:    parseTime("2005-05-14 21:07:03.111"),
			AccessTime: parseTime("2005-05-14 21:07:04.222"),
			ChangeTime: parseTime("2005-05-14 21:07:05.333"),
			ExtendedAttributes: []data.ExtendedAttribute{
				{"user.foo", []byte("bar")},
			},
		},
		{
			Name:       "testdirectory",
			Type:       data.NodeTypeDir,
			Mode:       0755,
			ModTime:    parseTime("2005-05-14 21:07:03.111"),
			AccessTime: parseTime("2005-05-14 21:07:04.222"),
			ChangeTime: parseTime("2005-05-14 21:07:05.333"),
			ExtendedAttributes: []data.ExtendedAttribute{
				{"user.foo", []byte("bar")},
			},
		},
	}
	for _, testNode := range expectedNodes {
		testPath, node := restoreAndGetNode(t, tempDir, &testNode, false)

		var handle windows.Handle
		var err error
		utf16Path := windows.StringToUTF16Ptr(testPath)
		if node.Type == data.NodeTypeFile || node.Type == data.NodeTypeDir {
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
