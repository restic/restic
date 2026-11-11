//go:build !windows

package fs

import (
	"os"
	"os/user"
	"slices"
	"strconv"
	"sync"

	"github.com/restic/restic/internal/data"
	"golang.org/x/sys/unix"
)

// nodeOwnershipNeedsRestore reports whether the owner and group stored in node
// differ from those of the file described by fi, mirroring what lchown would
// change, and whether the current process can apply such a change.
func nodeOwnershipNeedsRestore(node *data.Node, fi *ExtendedFileInfo, ownershipByName bool) (differs, appliable bool) {
	uid, gid := node.UID, node.GID
	if ownershipByName {
		uid = lookupUid(node.User)
		gid = lookupGid(node.Group)
	}

	if fi.UID == uid && fi.GID == gid {
		return false, true
	}

	if os.Geteuid() == 0 {
		return true, true
	}

	// An unprivileged process can only change the group of a file it owns to a
	// group it is a member of. Everything else fails with a permission error,
	// which NodeRestoreMetadata suppresses for unprivileged users.
	euid := uint32(os.Geteuid())
	appliable = fi.UID == euid && uid == fi.UID && memberOfGroup(gid)
	return true, appliable
}

var (
	currentUserGroupIDs     []uint32
	currentUserGroupIDsOnce sync.Once
)

// memberOfGroup reports whether the current user is a member of the group gid.
func memberOfGroup(gid uint32) bool {
	currentUserGroupIDsOnce.Do(func() {
		u, err := user.Current()
		if err != nil {
			return
		}
		gids, err := u.GroupIds()
		if err != nil {
			return
		}
		for _, g := range gids {
			if id, err := strconv.ParseUint(g, 10, 32); err == nil {
				currentUserGroupIDs = append(currentUserGroupIDs, uint32(id))
			}
		}
	})

	return slices.Contains(currentUserGroupIDs, gid)
}

// canModifyMetadata reports whether the current process can change the
// metadata (permissions, timestamps, extended attributes) of the file
// described by fi.
func canModifyMetadata(fi *ExtendedFileInfo) bool {
	return os.Geteuid() == 0 || fi.UID == uint32(os.Geteuid())
}

// canModifyXattrs reports whether the current process can change the extended
// attributes of the file at path. Ownership is sufficient but not required:
// setting or removing user.* attributes needs write permission on the file or
// directory. Only a sticky directory additionally limits modifications to its
// owner, mirroring xattr_permission in the Linux kernel.
func canModifyXattrs(path string, fi *ExtendedFileInfo) bool {
	if canModifyMetadata(fi) {
		return true
	}
	if fi.Mode&os.ModeDir != 0 && fi.Mode&os.ModeSticky != 0 {
		return false
	}
	return unix.Access(path, unix.W_OK) == nil
}

// nodeGenericAttrsNeedRestore is a no-op: on unix, generic attributes are only
// used to warn about attributes that cannot be restored.
func nodeGenericAttrsNeedRestore(_ *data.Node, _ string, _ *ExtendedFileInfo) (differs, appliable bool) {
	return false, false
}
