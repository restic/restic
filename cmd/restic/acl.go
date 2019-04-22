package main

// Adapted from https://github.com/maxymania/go-system/blob/master/posix_acl/posix_acl.go

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

const (
	aclUserOwner  = 0x0001
	aclUser       = 0x0002
	aclGroupOwner = 0x0004
	aclGroup      = 0x0008
	aclMask       = 0x0010
	aclOthers     = 0x0020
)

type aclSID uint64

type aclElem struct {
	Tag  uint16
	Perm uint16
	ID   uint32
}

type acl struct {
	Version uint32
	List    []aclElement
}

type aclElement struct {
	aclSID
	Perm uint16
}

func (a *aclSID) setUID(uid uint32) {
	*a = aclSID(uid) | (aclUser << 32)
}
func (a *aclSID) setGID(gid uint32) {
	*a = aclSID(gid) | (aclGroup << 32)
}

func (a *aclSID) setType(tp int) {
	*a = aclSID(tp) << 32
}

func (a aclSID) getType() int {
	return int(a >> 32)
}
func (a aclSID) getID() uint32 {
	return uint32(a & 0xffffffff)
}
func (a aclSID) String() string {
	switch a >> 32 {
	case aclUserOwner:
		return "user::"
	case aclUser:
		return fmt.Sprintf("user:%v:", a.getID())
	case aclGroupOwner:
		return "group::"
	case aclGroup:
		return fmt.Sprintf("group:%v:", a.getID())
	case aclMask:
		return "mask::"
	case aclOthers:
		return "other::"
	}
	return "?:"
}

func (a aclElement) String() string {
	str := ""
	if (a.Perm & 4) != 0 {
		str += "r"
	} else {
		str += "-"
	}
	if (a.Perm & 2) != 0 {
		str += "w"
	} else {
		str += "-"
	}
	if (a.Perm & 1) != 0 {
		str += "x"
	} else {
		str += "-"
	}
	return fmt.Sprintf("%v%v", a.aclSID, str)
}

func (a *acl) decode(xattr []byte) {
	var elem aclElement
	ae := new(aclElem)
	nr := bytes.NewReader(xattr)
	e := binary.Read(nr, binary.LittleEndian, &a.Version)
	if e != nil {
		a.Version = 0
		return
	}
	if len(a.List) > 0 {
		a.List = a.List[:0]
	}
	for binary.Read(nr, binary.LittleEndian, ae) == nil {
		elem.aclSID = (aclSID(ae.Tag) << 32) | aclSID(ae.ID)
		elem.Perm = ae.Perm
		a.List = append(a.List, elem)
	}
}

func (a *acl) encode() []byte {
	buf := new(bytes.Buffer)
	ae := new(aclElem)
	binary.Write(buf, binary.LittleEndian, &a.Version)
	for _, elem := range a.List {
		ae.Tag = uint16(elem.getType())
		ae.Perm = elem.Perm
		ae.ID = elem.getID()
		binary.Write(buf, binary.LittleEndian, ae)
	}
	return buf.Bytes()
}

func (a *acl) String() string {
	var finalacl string
	for _, acl := range a.List {
		finalacl += acl.String() + "\n"
	}
	return finalacl
}
