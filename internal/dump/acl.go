package dump

import (
	"encoding/binary"
	"errors"
	"strconv"
)

const (
	// Permissions
	aclPermRead    = 0x4
	aclPermWrite   = 0x2
	aclPermExecute = 0x1

	// Tags
	aclTagUserObj  = 0x01 // Owner.
	aclTagUser     = 0x02
	aclTagGroupObj = 0x04 // Owning group.
	aclTagGroup    = 0x08
	aclTagMask     = 0x10
	aclTagOther    = 0x20
)

// formatLinuxACL converts a Linux ACL from its binary format to the POSIX.1e
// long text format.
//
// User and group IDs are printed in decimal, because we may be dumping
// a snapshot from a different machine.
//
// https://man7.org/linux/man-pages/man5/acl.5.html
// https://savannah.nongnu.org/projects/acl
// https://simson.net/ref/1997/posix_1003.1e-990310.pdf
func formatLinuxACL(acl []byte) (string, error) {
	if len(acl)-4 < 0 || (len(acl)-4)%8 != 0 {
		return "", errors.New("wrong length")
	}
	version := binary.LittleEndian.Uint32(acl)
	if version != 2 {
		return "", errors.New("unsupported ACL format version")
	}
	acl = acl[4:]

	text := make([]byte, 0, 2*len(acl))

	for ; len(acl) >= 8; acl = acl[8:] {
		tag := binary.LittleEndian.Uint16(acl)
		perm := binary.LittleEndian.Uint16(acl[2:])
		id := binary.LittleEndian.Uint32(acl[4:])

		switch tag {
		case aclTagUserObj:
			text = append(text, "user:"...)
		case aclTagUser:
			text = append(text, "user:"...)
			text = strconv.AppendUint(text, uint64(id), 10)
		case aclTagGroupObj:
			text = append(text, "group:"...)
		case aclTagGroup:
			text = append(text, "group:"...)
			text = strconv.AppendUint(text, uint64(id), 10)
		case aclTagMask:
			text = append(text, "mask:"...)
		case aclTagOther:
			text = append(text, "other:"...)
		default:
			return "", errors.New("unknown tag")
		}
		text = append(text, ':')
		text = append(text, aclPermText(perm)...)
		text = append(text, '\n')
	}

	return string(text), nil
}

func aclPermText(p uint16) []byte {
	s := []byte("---")
	if p&aclPermRead != 0 {
		s[0] = 'r'
	}
	if p&aclPermWrite != 0 {
		s[1] = 'w'
	}
	if p&aclPermExecute != 0 {
		s[2] = 'x'
	}
	return s
}
