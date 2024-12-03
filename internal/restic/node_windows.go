package restic

import (
	"encoding/json"
	"os"
	"reflect"
	"runtime"
	"syscall"

	"github.com/restic/restic/internal/debug"
)

const AdsSeparator = "|"

// WindowsAttributes are the genericAttributes for Windows OS
type WindowsAttributes struct {
	// CreationTime is used for storing creation time for windows files.
	CreationTime *syscall.Filetime `generic:"creation_time"`
	// FileAttributes is used for storing file attributes for windows files.
	FileAttributes *uint32 `generic:"file_attributes"`
	// SecurityDescriptor is used for storing security descriptors which includes
	// owner, group, discretionary access control list (DACL), system access control list (SACL)
	SecurityDescriptor *[]byte `generic:"security_descriptor"`
	// HasADS is used for storing if a file has an Alternate Data Stream attached to it.
	HasADS *[]string `generic:"has_ads"`
	// IsADS is used for storing if a file is an Alternate Data Stream and is attached to (child of) a file in the value.
	// The file in the value will be a file which has a generic attribute TypeHasADS.
	IsADS *bool `generic:"is_ads"`
}

// windowsAttrsToGenericAttributes converts the WindowsAttributes to a generic attributes map using reflection
func WindowsAttrsToGenericAttributes(windowsAttributes WindowsAttributes) (attrs map[GenericAttributeType]json.RawMessage, err error) {
	// Get the value of the WindowsAttributes
	windowsAttributesValue := reflect.ValueOf(windowsAttributes)
	return OSAttrsToGenericAttributes(reflect.TypeOf(windowsAttributes), &windowsAttributesValue, runtime.GOOS)
}

// IsMainFile indicates if this is the main file and not a secondary file like an ads stream.
// This is used for functionalities we want to skip for secondary (ads) files.
// Eg. For Windows we do not want to count the secondary files
func (node Node) IsMainFile() bool {
	return string(node.GenericAttributes[TypeIsADS]) != "true"
}

// RemoveExtraStreams removes any extra streams on the file which are not present in the
// backed up state in the generic attribute TypeHasAds.
func (node Node) RemoveExtraStreams(path string) {
	success, existingStreams, _ := GetADStreamNames(path)
	if success {
		var adsValues []string

		hasAdsBytes := node.GenericAttributes[TypeHasADS]
		if hasAdsBytes != nil {
			var adsArray []string
			err := json.Unmarshal(hasAdsBytes, &adsArray)
			if err == nil {
				adsValues = adsArray
			}
		}

		extraStreams := filterItems(adsValues, existingStreams)
		for _, extraStream := range extraStreams {
			streamToRemove := path + extraStream
			err := os.Remove(streamToRemove)
			if err != nil {
				debug.Log("Error removing stream: %s : %s", streamToRemove, err)
			}
		}
	}
}

// filterItems filters out which items are in evalArray which are not in referenceArray.
func filterItems(referenceArray, evalArray []string) (result []string) {
	// Create a map to store elements of referenceArray for fast lookup
	referenceArrayMap := make(map[string]bool)
	for _, item := range referenceArray {
		referenceArrayMap[item] = true
	}

	// Iterate through elements of evalArray
	for _, item := range evalArray {
		// Check if the item is not in referenceArray
		if !referenceArrayMap[item] {
			// Append to the result array
			result = append(result, item)
		}
	}
	return result
}
