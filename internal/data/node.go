package data

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/restic"

	"github.com/restic/restic/internal/debug"
)

// ExtendedAttribute is a tuple storing the xattr name and value for various filesystems.
type ExtendedAttribute struct {
	Name  string `json:"name"`
	Value []byte `json:"value"`
}

// GenericAttributeType can be used for OS specific functionalities by defining specific types
// in node.go to be used by the specific node_xx files.
// OS specific attribute types should follow the convention <OS>Attributes.
// GenericAttributeTypes should follow the convention <OS specific attribute type>.<attribute name>
// The attributes in OS specific attribute types must be pointers as we want to distinguish nil values
// and not create GenericAttributes for them.
type GenericAttributeType string

// OSType is the type created to represent each specific OS
type OSType string

const (
	// When new GenericAttributeType are defined, they must be added in the init function as well.

	// Below are windows specific attributes.

	// TypeCreationTime is the GenericAttributeType used for storing creation time for windows files within the generic attributes map.
	TypeCreationTime GenericAttributeType = "windows.creation_time"
	// TypeFileAttributes is the GenericAttributeType used for storing file attributes for windows files within the generic attributes map.
	TypeFileAttributes GenericAttributeType = "windows.file_attributes"
	// TypeSecurityDescriptor is the GenericAttributeType used for storing security descriptors including owner, group, discretionary access control list (DACL), system access control list (SACL)) for windows files within the generic attributes map.
	TypeSecurityDescriptor GenericAttributeType = "windows.security_descriptor"

	// Generic Attributes for other OS types should be defined here.
)

// init is called when the package is initialized. Any new GenericAttributeTypes being created must be added here as well.
func init() {
	storeGenericAttributeType(TypeCreationTime, TypeFileAttributes, TypeSecurityDescriptor)
}

// genericAttributesForOS maintains a map of known genericAttributesForOS to the OSType
var genericAttributesForOS = map[GenericAttributeType]OSType{}

// storeGenericAttributeType adds and entry in genericAttributesForOS map
func storeGenericAttributeType(attributeTypes ...GenericAttributeType) {
	for _, attributeType := range attributeTypes {
		// Get the OS attribute type from the GenericAttributeType
		osAttributeName := strings.Split(string(attributeType), ".")[0]
		genericAttributesForOS[attributeType] = OSType(osAttributeName)
	}
}

type NodeType string

var (
	NodeTypeFile      = NodeType("file")
	NodeTypeDir       = NodeType("dir")
	NodeTypeSymlink   = NodeType("symlink")
	NodeTypeDev       = NodeType("dev")
	NodeTypeCharDev   = NodeType("chardev")
	NodeTypeFifo      = NodeType("fifo")
	NodeTypeSocket    = NodeType("socket")
	NodeTypeIrregular = NodeType("irregular")
	NodeTypeInvalid   = NodeType("")
)

// Node is a file, directory or other item in a backup.
type Node struct {
	Name       string      `json:"name"`
	Type       NodeType    `json:"type"`
	Mode       os.FileMode `json:"mode,omitempty"`
	ModTime    time.Time   `json:"mtime,omitempty"`
	AccessTime time.Time   `json:"atime,omitempty"`
	ChangeTime time.Time   `json:"ctime,omitempty"`
	UID        uint32      `json:"uid"`
	GID        uint32      `json:"gid"`
	User       string      `json:"user,omitempty"`
	Group      string      `json:"group,omitempty"`
	Inode      uint64      `json:"inode,omitempty"`
	DeviceID   uint64      `json:"device_id,omitempty"` // device id of the file, stat.st_dev, only stored for hardlinks
	Size       uint64      `json:"size,omitempty"`
	Links      uint64      `json:"links,omitempty"`
	LinkTarget string      `json:"linktarget,omitempty"`
	// implicitly base64-encoded field. Only used while encoding, `linktarget_raw` will overwrite LinkTarget if present.
	// This allows storing arbitrary byte-sequences, which are possible as symlink targets on unix systems,
	// as LinkTarget without breaking backwards-compatibility.
	// Must only be set of the linktarget cannot be encoded as valid utf8.
	LinkTargetRaw      []byte                                   `json:"linktarget_raw,omitempty"`
	ExtendedAttributes []ExtendedAttribute                      `json:"extended_attributes,omitempty"`
	GenericAttributes  map[GenericAttributeType]json.RawMessage `json:"generic_attributes,omitempty"`
	Device             uint64                                   `json:"device,omitempty"` // in case of Type == "dev", stat.st_rdev
	Content            restic.IDs                               `json:"content"`
	Subtree            *restic.ID                               `json:"subtree,omitempty"`

	Error string `json:"error,omitempty"`

	Path string `json:"-"`
}

// Nodes is a slice of nodes that can be sorted.
type Nodes []*Node

func (n Nodes) Len() int           { return len(n) }
func (n Nodes) Less(i, j int) bool { return n[i].Name < n[j].Name }
func (n Nodes) Swap(i, j int)      { n[i], n[j] = n[j], n[i] }

func (node Node) String() string {
	var mode os.FileMode
	switch node.Type {
	case NodeTypeFile:
		mode = 0
	case NodeTypeDir:
		mode = os.ModeDir
	case NodeTypeSymlink:
		mode = os.ModeSymlink
	case NodeTypeDev:
		mode = os.ModeDevice
	case NodeTypeCharDev:
		mode = os.ModeDevice | os.ModeCharDevice
	case NodeTypeFifo:
		mode = os.ModeNamedPipe
	case NodeTypeSocket:
		mode = os.ModeSocket
	}

	return fmt.Sprintf("%s %5d %5d %6d %s %s",
		mode|node.Mode, node.UID, node.GID, node.Size, node.ModTime, node.Name)
}

// GetExtendedAttribute gets the extended attribute.
func (node Node) GetExtendedAttribute(a string) []byte {
	for _, attr := range node.ExtendedAttributes {
		if attr.Name == a {
			return attr.Value
		}
	}
	return nil
}

// FixTime returns a time.Time which can safely be used to marshal as JSON. If
// the timestamp is earlier than year zero, the year is set to zero. In the same
// way, if the year is larger than 9999, the year is set to 9999. Other than
// the year nothing is changed.
func FixTime(t time.Time) time.Time {
	switch {
	case t.Year() < 0000:
		return t.AddDate(-t.Year(), 0, 0)
	case t.Year() > 9999:
		return t.AddDate(-(t.Year() - 9999), 0, 0)
	default:
		return t
	}
}

func (node Node) MarshalJSON() ([]byte, error) {
	// make sure invalid timestamps for mtime and atime are converted to
	// something we can actually save.
	node.ModTime = FixTime(node.ModTime)
	node.AccessTime = FixTime(node.AccessTime)
	node.ChangeTime = FixTime(node.ChangeTime)

	type nodeJSON Node
	nj := nodeJSON(node)
	name := strconv.Quote(node.Name)
	nj.Name = name[1 : len(name)-1]
	if nj.LinkTargetRaw != nil {
		panic("LinkTargetRaw must not be set manually")
	}
	if !utf8.ValidString(node.LinkTarget) {
		// store raw bytes if invalid utf8
		nj.LinkTargetRaw = []byte(node.LinkTarget)
	}

	return json.Marshal(nj)
}

func (node *Node) UnmarshalJSON(data []byte) error {
	type nodeJSON Node
	nj := (*nodeJSON)(node)

	err := json.Unmarshal(data, nj)
	if err != nil {
		return errors.Wrap(err, "Unmarshal")
	}

	nj.Name, err = strconv.Unquote(`"` + nj.Name + `"`)
	if err != nil {
		return errors.Wrap(err, "Unquote")
	}
	if nj.LinkTargetRaw != nil {
		nj.LinkTarget = string(nj.LinkTargetRaw)
		nj.LinkTargetRaw = nil
	}
	return nil
}

func (node Node) Equals(other Node) bool {
	if node.Name != other.Name {
		return false
	}
	if node.Type != other.Type {
		return false
	}
	if node.Mode != other.Mode {
		return false
	}
	if !node.ModTime.Equal(other.ModTime) {
		return false
	}
	if !node.AccessTime.Equal(other.AccessTime) {
		return false
	}
	if !node.ChangeTime.Equal(other.ChangeTime) {
		return false
	}
	if node.UID != other.UID {
		return false
	}
	if node.GID != other.GID {
		return false
	}
	if node.User != other.User {
		return false
	}
	if node.Group != other.Group {
		return false
	}
	if node.Inode != other.Inode {
		return false
	}
	if node.DeviceID != other.DeviceID {
		return false
	}
	if node.Size != other.Size {
		return false
	}
	if node.Links != other.Links {
		return false
	}
	if node.LinkTarget != other.LinkTarget {
		return false
	}
	if node.Device != other.Device {
		return false
	}
	if !node.sameContent(other) {
		return false
	}
	if !node.sameExtendedAttributes(other) {
		return false
	}
	if !node.sameGenericAttributes(other) {
		return false
	}
	if node.Subtree != nil {
		if other.Subtree == nil {
			return false
		}

		if !node.Subtree.Equal(*other.Subtree) {
			return false
		}
	} else {
		if other.Subtree != nil {
			return false
		}
	}
	if node.Error != other.Error {
		return false
	}

	return true
}

func (node Node) sameContent(other Node) bool {
	if node.Content == nil {
		return other.Content == nil
	}

	if other.Content == nil {
		return false
	}

	if len(node.Content) != len(other.Content) {
		return false
	}

	for i := 0; i < len(node.Content); i++ {
		if !node.Content[i].Equal(other.Content[i]) {
			return false
		}
	}
	return true
}

func (node Node) sameExtendedAttributes(other Node) bool {
	ln := len(node.ExtendedAttributes)
	lo := len(other.ExtendedAttributes)
	if ln != lo {
		return false
	} else if ln == 0 {
		// This means lo is also of length 0
		return true
	}

	// build a set of all attributes that node has
	type mapvalue struct {
		value   []byte
		present bool
	}
	attributes := make(map[string]mapvalue)
	for _, attr := range node.ExtendedAttributes {
		attributes[attr.Name] = mapvalue{value: attr.Value}
	}

	for _, attr := range other.ExtendedAttributes {
		v, ok := attributes[attr.Name]
		if !ok {
			// extended attribute is not set for node
			debug.Log("other node has attribute %v, which is not present in node", attr.Name)
			return false

		}

		if !bytes.Equal(v.value, attr.Value) {
			// attribute has different value
			debug.Log("attribute %v has different value", attr.Name)
			return false
		}

		// remember that this attribute is present in other.
		v.present = true
		attributes[attr.Name] = v
	}

	// check for attributes that are not present in other
	for name, v := range attributes {
		if !v.present {
			debug.Log("attribute %v not present in other node", name)
			return false
		}
	}

	return true
}

func (node Node) sameGenericAttributes(other Node) bool {
	return deepEqual(node.GenericAttributes, other.GenericAttributes)
}

func deepEqual(map1, map2 map[GenericAttributeType]json.RawMessage) bool {
	// Check if the maps have the same number of keys
	if len(map1) != len(map2) {
		return false
	}

	// Iterate over each key-value pair in map1
	for key, value1 := range map1 {
		// Check if the key exists in map2
		value2, ok := map2[key]
		if !ok {
			return false
		}

		// Check if the JSON.RawMessage values are equal byte by byte
		if !bytes.Equal(value1, value2) {
			return false
		}
	}

	return true
}

// HandleUnknownGenericAttributesFound is used for handling and distinguing between scenarios related to future versions and cross-OS repositories
func HandleUnknownGenericAttributesFound(unknownAttribs []GenericAttributeType, warn func(msg string)) {
	for _, unknownAttrib := range unknownAttribs {
		handleUnknownGenericAttributeFound(unknownAttrib, warn)
	}
}

// handleUnknownGenericAttributeFound is used for handling and distinguing between scenarios related to future versions and cross-OS repositories
func handleUnknownGenericAttributeFound(genericAttributeType GenericAttributeType, warn func(msg string)) {
	if checkGenericAttributeNameNotHandledAndPut(genericAttributeType) {
		// Print the unique error only once for a given execution
		os, exists := genericAttributesForOS[genericAttributeType]

		if exists {
			// If genericAttributesForOS contains an entry but we still got here, it means the specific node_xx.go for the current OS did not handle it and the repository may have been originally created on a different OS.
			// The fact that node.go knows about the attribute, means it is not a new attribute. This may be a common situation if a repo is used across OSs.
			debug.Log("Ignoring a generic attribute found in the repository: %s which may not be compatible with your OS. Compatible OS: %s", genericAttributeType, os)
		} else {
			// If genericAttributesForOS in node.go does not know about this attribute, then the repository may have been created by a newer version which has a newer GenericAttributeType.
			warn(fmt.Sprintf("Found an unrecognized generic attribute in the repository: %s. You may need to upgrade to latest version of restic.", genericAttributeType))
		}
	}
}

// HandleAllUnknownGenericAttributesFound performs validations for all generic attributes of a node.
// This is not used on windows currently because windows has handling for generic attributes.
func HandleAllUnknownGenericAttributesFound(attributes map[GenericAttributeType]json.RawMessage, warn func(msg string)) error {
	for name := range attributes {
		handleUnknownGenericAttributeFound(name, warn)
	}
	return nil
}

var unknownGenericAttributesHandlingHistory sync.Map

// checkGenericAttributeNameNotHandledAndPut checks if the GenericAttributeType name entry
// already exists and puts it in the map if not.
func checkGenericAttributeNameNotHandledAndPut(value GenericAttributeType) bool {
	// If Key doesn't exist, put the value and return true because it is not already handled
	_, exists := unknownGenericAttributesHandlingHistory.LoadOrStore(value, "")
	// Key exists, then it is already handled so return false
	return !exists
}

// The functions below are common helper functions which can be used for generic attributes support
// across different OS.

// GenericAttributesToOSAttrs gets the os specific attribute from the generic attribute using reflection
func GenericAttributesToOSAttrs(attrs map[GenericAttributeType]json.RawMessage, attributeType reflect.Type, attributeValuePtr *reflect.Value, keyPrefix string) (unknownAttribs []GenericAttributeType, err error) {
	attributeValue := *attributeValuePtr

	for key, rawMsg := range attrs {
		found := false
		for i := 0; i < attributeType.NumField(); i++ {
			if getFQKeyByIndex(attributeType, i, keyPrefix) == key {
				found = true
				fieldValue := attributeValue.Field(i)
				// For directly supported types, use json.Unmarshal directly
				if err := json.Unmarshal(rawMsg, fieldValue.Addr().Interface()); err != nil {
					return unknownAttribs, errors.Wrap(err, "Unmarshal")
				}
				break
			}
		}
		if !found {
			unknownAttribs = append(unknownAttribs, key)
		}
	}
	return unknownAttribs, nil
}

// getFQKey gets the fully qualified key for the field
func getFQKey(field reflect.StructField, keyPrefix string) GenericAttributeType {
	return GenericAttributeType(fmt.Sprintf("%s.%s", keyPrefix, field.Tag.Get("generic")))
}

// getFQKeyByIndex gets the fully qualified key for the field index
func getFQKeyByIndex(attributeType reflect.Type, index int, keyPrefix string) GenericAttributeType {
	return getFQKey(attributeType.Field(index), keyPrefix)
}

// OSAttrsToGenericAttributes gets the generic attribute from the os specific attribute using reflection
func OSAttrsToGenericAttributes(attributeType reflect.Type, attributeValuePtr *reflect.Value, keyPrefix string) (attrs map[GenericAttributeType]json.RawMessage, err error) {
	attributeValue := *attributeValuePtr
	attrs = make(map[GenericAttributeType]json.RawMessage)

	// Iterate over the fields of the struct
	for i := 0; i < attributeType.NumField(); i++ {
		field := attributeType.Field(i)

		// Get the field value using reflection
		fieldValue := attributeValue.FieldByName(field.Name)

		// Check if the field is nil
		if fieldValue.IsNil() {
			// If it's nil, skip this field
			continue
		}

		// Marshal the field value into a json.RawMessage
		var fieldBytes []byte
		if fieldBytes, err = json.Marshal(fieldValue.Interface()); err != nil {
			return attrs, errors.Wrap(err, "Marshal")
		}

		// Insert the field into the map
		attrs[getFQKey(field, keyPrefix)] = fieldBytes
	}
	return attrs, nil
}
