package restic

import (
	"encoding/json"
	"reflect"
	"runtime"
	"syscall"
)

// WindowsAttributes are the genericAttributes for Windows OS
type WindowsAttributes struct {
	// CreationTime is used for storing creation time for windows files.
	CreationTime *syscall.Filetime `generic:"creation_time"`
	// FileAttributes is used for storing file attributes for windows files.
	FileAttributes *uint32 `generic:"file_attributes"`
	// SecurityDescriptor is used for storing security descriptors which includes
	// owner, group, discretionary access control list (DACL), system access control list (SACL)
	SecurityDescriptor *[]byte `generic:"security_descriptor"`
}

// WindowsAttrsToGenericAttributes converts the WindowsAttributes to a generic attributes map using reflection
func WindowsAttrsToGenericAttributes(windowsAttributes WindowsAttributes) (attrs map[GenericAttributeType]json.RawMessage, err error) {
	// Get the value of the WindowsAttributes
	windowsAttributesValue := reflect.ValueOf(windowsAttributes)
	return OSAttrsToGenericAttributes(reflect.TypeOf(windowsAttributes), &windowsAttributesValue, runtime.GOOS)
}
