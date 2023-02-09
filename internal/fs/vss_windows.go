//go:build windows
// +build windows

package fs

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"unsafe"

	ole "github.com/go-ole/go-ole"
	"github.com/restic/restic/internal/errors"
	"golang.org/x/sys/windows"
)

// HRESULT is a custom type for the windows api HRESULT type.
type HRESULT uint

// HRESULT constant values necessary for using VSS api.
const (
	S_OK                                            HRESULT = 0x00000000
	E_ACCESSDENIED                                  HRESULT = 0x80070005
	E_OUTOFMEMORY                                   HRESULT = 0x8007000E
	E_INVALIDARG                                    HRESULT = 0x80070057
	VSS_E_BAD_STATE                                 HRESULT = 0x80042301
	VSS_E_UNEXPECTED                                HRESULT = 0x80042302
	VSS_E_PROVIDER_ALREADY_REGISTERED               HRESULT = 0x80042303
	VSS_E_PROVIDER_NOT_REGISTERED                   HRESULT = 0x80042304
	VSS_E_PROVIDER_VETO                             HRESULT = 0x80042306
	VSS_E_PROVIDER_IN_USE                           HRESULT = 0x80042307
	VSS_E_OBJECT_NOT_FOUND                          HRESULT = 0x80042308
	VSS_E_VOLUME_NOT_SUPPORTED                      HRESULT = 0x8004230C
	VSS_E_VOLUME_NOT_SUPPORTED_BY_PROVIDER          HRESULT = 0x8004230E
	VSS_E_OBJECT_ALREADY_EXISTS                     HRESULT = 0x8004230D
	VSS_E_UNEXPECTED_PROVIDER_ERROR                 HRESULT = 0x8004230F
	VSS_E_CORRUPT_XML_DOCUMENT                      HRESULT = 0x80042310
	VSS_E_INVALID_XML_DOCUMENT                      HRESULT = 0x80042311
	VSS_E_MAXIMUM_NUMBER_OF_VOLUMES_REACHED         HRESULT = 0x80042312
	VSS_E_FLUSH_WRITES_TIMEOUT                      HRESULT = 0x80042313
	VSS_E_HOLD_WRITES_TIMEOUT                       HRESULT = 0x80042314
	VSS_E_UNEXPECTED_WRITER_ERROR                   HRESULT = 0x80042315
	VSS_E_SNAPSHOT_SET_IN_PROGRESS                  HRESULT = 0x80042316
	VSS_E_MAXIMUM_NUMBER_OF_SNAPSHOTS_REACHED       HRESULT = 0x80042317
	VSS_E_WRITER_INFRASTRUCTURE                     HRESULT = 0x80042318
	VSS_E_WRITER_NOT_RESPONDING                     HRESULT = 0x80042319
	VSS_E_WRITER_ALREADY_SUBSCRIBED                 HRESULT = 0x8004231A
	VSS_E_UNSUPPORTED_CONTEXT                       HRESULT = 0x8004231B
	VSS_E_VOLUME_IN_USE                             HRESULT = 0x8004231D
	VSS_E_MAXIMUM_DIFFAREA_ASSOCIATIONS_REACHED     HRESULT = 0x8004231E
	VSS_E_INSUFFICIENT_STORAGE                      HRESULT = 0x8004231F
	VSS_E_NO_SNAPSHOTS_IMPORTED                     HRESULT = 0x80042320
	VSS_E_SOME_SNAPSHOTS_NOT_IMPORTED               HRESULT = 0x80042321
	VSS_E_MAXIMUM_NUMBER_OF_REMOTE_MACHINES_REACHED HRESULT = 0x80042322
	VSS_E_REMOTE_SERVER_UNAVAILABLE                 HRESULT = 0x80042323
	VSS_E_REMOTE_SERVER_UNSUPPORTED                 HRESULT = 0x80042324
	VSS_E_REVERT_IN_PROGRESS                        HRESULT = 0x80042325
	VSS_E_REVERT_VOLUME_LOST                        HRESULT = 0x80042326
	VSS_E_REBOOT_REQUIRED                           HRESULT = 0x80042327
	VSS_E_TRANSACTION_FREEZE_TIMEOUT                HRESULT = 0x80042328
	VSS_E_TRANSACTION_THAW_TIMEOUT                  HRESULT = 0x80042329
	VSS_E_VOLUME_NOT_LOCAL                          HRESULT = 0x8004232D
	VSS_E_CLUSTER_TIMEOUT                           HRESULT = 0x8004232E
	VSS_E_WRITERERROR_INCONSISTENTSNAPSHOT          HRESULT = 0x800423F0
	VSS_E_WRITERERROR_OUTOFRESOURCES                HRESULT = 0x800423F1
	VSS_E_WRITERERROR_TIMEOUT                       HRESULT = 0x800423F2
	VSS_E_WRITERERROR_RETRYABLE                     HRESULT = 0x800423F3
	VSS_E_WRITERERROR_NONRETRYABLE                  HRESULT = 0x800423F4
	VSS_E_WRITERERROR_RECOVERY_FAILED               HRESULT = 0x800423F5
	VSS_E_BREAK_REVERT_ID_FAILED                    HRESULT = 0x800423F6
	VSS_E_LEGACY_PROVIDER                           HRESULT = 0x800423F7
	VSS_E_MISSING_DISK                              HRESULT = 0x800423F8
	VSS_E_MISSING_HIDDEN_VOLUME                     HRESULT = 0x800423F9
	VSS_E_MISSING_VOLUME                            HRESULT = 0x800423FA
	VSS_E_AUTORECOVERY_FAILED                       HRESULT = 0x800423FB
	VSS_E_DYNAMIC_DISK_ERROR                        HRESULT = 0x800423FC
	VSS_E_NONTRANSPORTABLE_BCD                      HRESULT = 0x800423FD
	VSS_E_CANNOT_REVERT_DISKID                      HRESULT = 0x800423FE
	VSS_E_RESYNC_IN_PROGRESS                        HRESULT = 0x800423FF
	VSS_E_CLUSTER_ERROR                             HRESULT = 0x80042400
	VSS_E_UNSELECTED_VOLUME                         HRESULT = 0x8004232A
	VSS_E_SNAPSHOT_NOT_IN_SET                       HRESULT = 0x8004232B
	VSS_E_NESTED_VOLUME_LIMIT                       HRESULT = 0x8004232C
	VSS_E_NOT_SUPPORTED                             HRESULT = 0x8004232F
	VSS_E_WRITERERROR_PARTIAL_FAILURE               HRESULT = 0x80042336
	VSS_E_WRITER_STATUS_NOT_AVAILABLE               HRESULT = 0x80042409
)

// hresultToString maps a HRESULT value to a human readable string.
var hresultToString = map[HRESULT]string{
	S_OK:                                            "S_OK",
	E_ACCESSDENIED:                                  "E_ACCESSDENIED",
	E_OUTOFMEMORY:                                   "E_OUTOFMEMORY",
	E_INVALIDARG:                                    "E_INVALIDARG",
	VSS_E_BAD_STATE:                                 "VSS_E_BAD_STATE",
	VSS_E_UNEXPECTED:                                "VSS_E_UNEXPECTED",
	VSS_E_PROVIDER_ALREADY_REGISTERED:               "VSS_E_PROVIDER_ALREADY_REGISTERED",
	VSS_E_PROVIDER_NOT_REGISTERED:                   "VSS_E_PROVIDER_NOT_REGISTERED",
	VSS_E_PROVIDER_VETO:                             "VSS_E_PROVIDER_VETO",
	VSS_E_PROVIDER_IN_USE:                           "VSS_E_PROVIDER_IN_USE",
	VSS_E_OBJECT_NOT_FOUND:                          "VSS_E_OBJECT_NOT_FOUND",
	VSS_E_VOLUME_NOT_SUPPORTED:                      "VSS_E_VOLUME_NOT_SUPPORTED",
	VSS_E_VOLUME_NOT_SUPPORTED_BY_PROVIDER:          "VSS_E_VOLUME_NOT_SUPPORTED_BY_PROVIDER",
	VSS_E_OBJECT_ALREADY_EXISTS:                     "VSS_E_OBJECT_ALREADY_EXISTS",
	VSS_E_UNEXPECTED_PROVIDER_ERROR:                 "VSS_E_UNEXPECTED_PROVIDER_ERROR",
	VSS_E_CORRUPT_XML_DOCUMENT:                      "VSS_E_CORRUPT_XML_DOCUMENT",
	VSS_E_INVALID_XML_DOCUMENT:                      "VSS_E_INVALID_XML_DOCUMENT",
	VSS_E_MAXIMUM_NUMBER_OF_VOLUMES_REACHED:         "VSS_E_MAXIMUM_NUMBER_OF_VOLUMES_REACHED",
	VSS_E_FLUSH_WRITES_TIMEOUT:                      "VSS_E_FLUSH_WRITES_TIMEOUT",
	VSS_E_HOLD_WRITES_TIMEOUT:                       "VSS_E_HOLD_WRITES_TIMEOUT",
	VSS_E_UNEXPECTED_WRITER_ERROR:                   "VSS_E_UNEXPECTED_WRITER_ERROR",
	VSS_E_SNAPSHOT_SET_IN_PROGRESS:                  "VSS_E_SNAPSHOT_SET_IN_PROGRESS",
	VSS_E_MAXIMUM_NUMBER_OF_SNAPSHOTS_REACHED:       "VSS_E_MAXIMUM_NUMBER_OF_SNAPSHOTS_REACHED",
	VSS_E_WRITER_INFRASTRUCTURE:                     "VSS_E_WRITER_INFRASTRUCTURE",
	VSS_E_WRITER_NOT_RESPONDING:                     "VSS_E_WRITER_NOT_RESPONDING",
	VSS_E_WRITER_ALREADY_SUBSCRIBED:                 "VSS_E_WRITER_ALREADY_SUBSCRIBED",
	VSS_E_UNSUPPORTED_CONTEXT:                       "VSS_E_UNSUPPORTED_CONTEXT",
	VSS_E_VOLUME_IN_USE:                             "VSS_E_VOLUME_IN_USE",
	VSS_E_MAXIMUM_DIFFAREA_ASSOCIATIONS_REACHED:     "VSS_E_MAXIMUM_DIFFAREA_ASSOCIATIONS_REACHED",
	VSS_E_INSUFFICIENT_STORAGE:                      "VSS_E_INSUFFICIENT_STORAGE",
	VSS_E_NO_SNAPSHOTS_IMPORTED:                     "VSS_E_NO_SNAPSHOTS_IMPORTED",
	VSS_E_SOME_SNAPSHOTS_NOT_IMPORTED:               "VSS_E_SOME_SNAPSHOTS_NOT_IMPORTED",
	VSS_E_MAXIMUM_NUMBER_OF_REMOTE_MACHINES_REACHED: "VSS_E_MAXIMUM_NUMBER_OF_REMOTE_MACHINES_REACHED",
	VSS_E_REMOTE_SERVER_UNAVAILABLE:                 "VSS_E_REMOTE_SERVER_UNAVAILABLE",
	VSS_E_REMOTE_SERVER_UNSUPPORTED:                 "VSS_E_REMOTE_SERVER_UNSUPPORTED",
	VSS_E_REVERT_IN_PROGRESS:                        "VSS_E_REVERT_IN_PROGRESS",
	VSS_E_REVERT_VOLUME_LOST:                        "VSS_E_REVERT_VOLUME_LOST",
	VSS_E_REBOOT_REQUIRED:                           "VSS_E_REBOOT_REQUIRED",
	VSS_E_TRANSACTION_FREEZE_TIMEOUT:                "VSS_E_TRANSACTION_FREEZE_TIMEOUT",
	VSS_E_TRANSACTION_THAW_TIMEOUT:                  "VSS_E_TRANSACTION_THAW_TIMEOUT",
	VSS_E_VOLUME_NOT_LOCAL:                          "VSS_E_VOLUME_NOT_LOCAL",
	VSS_E_CLUSTER_TIMEOUT:                           "VSS_E_CLUSTER_TIMEOUT",
	VSS_E_WRITERERROR_INCONSISTENTSNAPSHOT:          "VSS_E_WRITERERROR_INCONSISTENTSNAPSHOT",
	VSS_E_WRITERERROR_OUTOFRESOURCES:                "VSS_E_WRITERERROR_OUTOFRESOURCES",
	VSS_E_WRITERERROR_TIMEOUT:                       "VSS_E_WRITERERROR_TIMEOUT",
	VSS_E_WRITERERROR_RETRYABLE:                     "VSS_E_WRITERERROR_RETRYABLE",
	VSS_E_WRITERERROR_NONRETRYABLE:                  "VSS_E_WRITERERROR_NONRETRYABLE",
	VSS_E_WRITERERROR_RECOVERY_FAILED:               "VSS_E_WRITERERROR_RECOVERY_FAILED",
	VSS_E_BREAK_REVERT_ID_FAILED:                    "VSS_E_BREAK_REVERT_ID_FAILED",
	VSS_E_LEGACY_PROVIDER:                           "VSS_E_LEGACY_PROVIDER",
	VSS_E_MISSING_DISK:                              "VSS_E_MISSING_DISK",
	VSS_E_MISSING_HIDDEN_VOLUME:                     "VSS_E_MISSING_HIDDEN_VOLUME",
	VSS_E_MISSING_VOLUME:                            "VSS_E_MISSING_VOLUME",
	VSS_E_AUTORECOVERY_FAILED:                       "VSS_E_AUTORECOVERY_FAILED",
	VSS_E_DYNAMIC_DISK_ERROR:                        "VSS_E_DYNAMIC_DISK_ERROR",
	VSS_E_NONTRANSPORTABLE_BCD:                      "VSS_E_NONTRANSPORTABLE_BCD",
	VSS_E_CANNOT_REVERT_DISKID:                      "VSS_E_CANNOT_REVERT_DISKID",
	VSS_E_RESYNC_IN_PROGRESS:                        "VSS_E_RESYNC_IN_PROGRESS",
	VSS_E_CLUSTER_ERROR:                             "VSS_E_CLUSTER_ERROR",
	VSS_E_UNSELECTED_VOLUME:                         "VSS_E_UNSELECTED_VOLUME",
	VSS_E_SNAPSHOT_NOT_IN_SET:                       "VSS_E_SNAPSHOT_NOT_IN_SET",
	VSS_E_NESTED_VOLUME_LIMIT:                       "VSS_E_NESTED_VOLUME_LIMIT",
	VSS_E_NOT_SUPPORTED:                             "VSS_E_NOT_SUPPORTED",
	VSS_E_WRITERERROR_PARTIAL_FAILURE:               "VSS_E_WRITERERROR_PARTIAL_FAILURE",
	VSS_E_WRITER_STATUS_NOT_AVAILABLE:               "VSS_E_WRITER_STATUS_NOT_AVAILABLE",
}

// Str converts a HRESULT to a human readable string.
func (h HRESULT) Str() string {
	if i, ok := hresultToString[h]; ok {
		return i
	}

	return "UNKNOWN"
}

// VssError encapsulates errors retruned from calling VSS api.
type vssError struct {
	text    string
	hresult HRESULT
}

// NewVssError creates a new VSS api error.
func newVssError(text string, hresult HRESULT) error {
	return &vssError{text: text, hresult: hresult}
}

// NewVssError creates a new VSS api error.
func newVssErrorIfResultNotOK(text string, hresult HRESULT) error {
	if hresult != S_OK {
		return newVssError(text, hresult)
	}
	return nil
}

// Error implements the error interface.
func (e *vssError) Error() string {
	return fmt.Sprintf("VSS error: %s: %s (%#x)", e.text, e.hresult.Str(), e.hresult)
}

// VssError encapsulates errors retruned from calling VSS api.
type vssTextError struct {
	text string
}

// NewVssTextError creates a new VSS api error.
func newVssTextError(text string) error {
	return &vssTextError{text: text}
}

// Error implements the error interface.
func (e *vssTextError) Error() string {
	return fmt.Sprintf("VSS error: %s", e.text)
}

// VssContext is a custom type for the windows api VssContext type.
type VssContext uint

// VssContext constant values necessary for using VSS api.
const (
	VSS_CTX_BACKUP VssContext = iota
	VSS_CTX_FILE_SHARE_BACKUP
	VSS_CTX_NAS_ROLLBACK
	VSS_CTX_APP_ROLLBACK
	VSS_CTX_CLIENT_ACCESSIBLE
	VSS_CTX_CLIENT_ACCESSIBLE_WRITERS
	VSS_CTX_ALL
)

// VssBackup is a custom type for the windows api VssBackup type.
type VssBackup uint

// VssBackup constant values necessary for using VSS api.
const (
	VSS_BT_UNDEFINED VssBackup = iota
	VSS_BT_FULL
	VSS_BT_INCREMENTAL
	VSS_BT_DIFFERENTIAL
	VSS_BT_LOG
	VSS_BT_COPY
	VSS_BT_OTHER
)

// VssObjectType is a custom type for the windows api VssObjectType type.
type VssObjectType uint

// VssObjectType constant values necessary for using VSS api.
const (
	VSS_OBJECT_UNKNOWN VssObjectType = iota
	VSS_OBJECT_NONE
	VSS_OBJECT_SNAPSHOT_SET
	VSS_OBJECT_SNAPSHOT
	VSS_OBJECT_PROVIDER
	VSS_OBJECT_TYPE_COUNT
)

// UUID_IVSS defines the GUID of IVssBackupComponents.
var UUID_IVSS = ole.NewGUID("{665c1d5f-c218-414d-a05d-7fef5f9d5c86}")

// IVssBackupComponents VSS api interface.
type IVssBackupComponents struct {
	ole.IUnknown
}

// IVssBackupComponentsVTable is the vtable for IVssBackupComponents.
type IVssBackupComponentsVTable struct {
	ole.IUnknownVtbl
	getWriterComponentsCount      uintptr
	getWriterComponents           uintptr
	initializeForBackup           uintptr
	setBackupState                uintptr
	initializeForRestore          uintptr
	setRestoreState               uintptr
	gatherWriterMetadata          uintptr
	getWriterMetadataCount        uintptr
	getWriterMetadata             uintptr
	freeWriterMetadata            uintptr
	addComponent                  uintptr
	prepareForBackup              uintptr
	abortBackup                   uintptr
	gatherWriterStatus            uintptr
	getWriterStatusCount          uintptr
	freeWriterStatus              uintptr
	getWriterStatus               uintptr
	setBackupSucceeded            uintptr
	setBackupOptions              uintptr
	setSelectedForRestore         uintptr
	setRestoreOptions             uintptr
	setAdditionalRestores         uintptr
	setPreviousBackupStamp        uintptr
	saveAsXML                     uintptr
	backupComplete                uintptr
	addAlternativeLocationMapping uintptr
	addRestoreSubcomponent        uintptr
	setFileRestoreStatus          uintptr
	addNewTarget                  uintptr
	setRangesFilePath             uintptr
	preRestore                    uintptr
	postRestore                   uintptr
	setContext                    uintptr
	startSnapshotSet              uintptr
	addToSnapshotSet              uintptr
	doSnapshotSet                 uintptr
	deleteSnapshots               uintptr
	importSnapshots               uintptr
	breakSnapshotSet              uintptr
	getSnapshotProperties         uintptr
	query                         uintptr
	isVolumeSupported             uintptr
	disableWriterClasses          uintptr
	enableWriterClasses           uintptr
	disableWriterInstances        uintptr
	exposeSnapshot                uintptr
	revertToSnapshot              uintptr
	queryRevertStatus             uintptr
}

// getVTable returns the vtable for IVssBackupComponents.
func (vss *IVssBackupComponents) getVTable() *IVssBackupComponentsVTable {
	return (*IVssBackupComponentsVTable)(unsafe.Pointer(vss.RawVTable))
}

// AbortBackup calls the equivalent VSS api.
func (vss *IVssBackupComponents) AbortBackup() error {
	result, _, _ := syscall.Syscall(vss.getVTable().abortBackup, 1,
		uintptr(unsafe.Pointer(vss)), 0, 0)

	return newVssErrorIfResultNotOK("AbortBackup() failed", HRESULT(result))
}

// InitializeForBackup calls the equivalent VSS api.
func (vss *IVssBackupComponents) InitializeForBackup() error {
	result, _, _ := syscall.Syscall(vss.getVTable().initializeForBackup, 2,
		uintptr(unsafe.Pointer(vss)), 0, 0)

	return newVssErrorIfResultNotOK("InitializeForBackup() failed", HRESULT(result))
}

// SetContext calls the equivalent VSS api.
func (vss *IVssBackupComponents) SetContext(context VssContext) error {
	result, _, _ := syscall.Syscall(vss.getVTable().setContext, 2,
		uintptr(unsafe.Pointer(vss)), uintptr(context), 0)

	return newVssErrorIfResultNotOK("SetContext() failed", HRESULT(result))
}

// GatherWriterMetadata calls the equivalent VSS api.
func (vss *IVssBackupComponents) GatherWriterMetadata() (*IVSSAsync, error) {
	var oleIUnknown *ole.IUnknown
	result, _, _ := syscall.Syscall(vss.getVTable().gatherWriterMetadata, 2,
		uintptr(unsafe.Pointer(vss)), uintptr(unsafe.Pointer(&oleIUnknown)), 0)

	err := newVssErrorIfResultNotOK("GatherWriterMetadata() failed", HRESULT(result))
	return vss.convertToVSSAsync(oleIUnknown, err)
}

// convertToVSSAsync looks up IVSSAsync interface if given result
// is a success.
func (vss *IVssBackupComponents) convertToVSSAsync(
	oleIUnknown *ole.IUnknown, err error) (*IVSSAsync, error) {
	if err != nil {
		return nil, err
	}

	comInterface, err := queryInterface(oleIUnknown, UIID_IVSS_ASYNC)
	if err != nil {
		return nil, err
	}

	iVssAsync := (*IVSSAsync)(unsafe.Pointer(comInterface))
	return iVssAsync, nil
}

// IsVolumeSupported calls the equivalent VSS api.
func (vss *IVssBackupComponents) IsVolumeSupported(volumeName string) (bool, error) {
	volumeNamePointer, err := syscall.UTF16PtrFromString(volumeName)
	if err != nil {
		panic(err)
	}

	var isSupportedRaw uint32
	var result uintptr

	if runtime.GOARCH == "386" {
		id := (*[4]uintptr)(unsafe.Pointer(ole.IID_NULL))

		result, _, _ = syscall.Syscall9(vss.getVTable().isVolumeSupported, 7,
			uintptr(unsafe.Pointer(vss)), id[0], id[1], id[2], id[3],
			uintptr(unsafe.Pointer(volumeNamePointer)), uintptr(unsafe.Pointer(&isSupportedRaw)), 0,
			0)
	} else {
		result, _, _ = syscall.Syscall6(vss.getVTable().isVolumeSupported, 4,
			uintptr(unsafe.Pointer(vss)), uintptr(unsafe.Pointer(ole.IID_NULL)),
			uintptr(unsafe.Pointer(volumeNamePointer)), uintptr(unsafe.Pointer(&isSupportedRaw)), 0,
			0)
	}

	var isSupported bool
	if isSupportedRaw == 0 {
		isSupported = false
	} else {
		isSupported = true
	}

	return isSupported, newVssErrorIfResultNotOK("IsVolumeSupported() failed", HRESULT(result))
}

// StartSnapshotSet calls the equivalent VSS api.
func (vss *IVssBackupComponents) StartSnapshotSet() (ole.GUID, error) {
	var snapshotSetID ole.GUID
	result, _, _ := syscall.Syscall(vss.getVTable().startSnapshotSet, 2,
		uintptr(unsafe.Pointer(vss)), uintptr(unsafe.Pointer(&snapshotSetID)), 0,
	)

	return snapshotSetID, newVssErrorIfResultNotOK("StartSnapshotSet() failed", HRESULT(result))
}

// AddToSnapshotSet calls the equivalent VSS api.
func (vss *IVssBackupComponents) AddToSnapshotSet(volumeName string, idSnapshot *ole.GUID) error {
	volumeNamePointer, err := syscall.UTF16PtrFromString(volumeName)
	if err != nil {
		panic(err)
	}

	var result uintptr = 0

	if runtime.GOARCH == "386" {
		id := (*[4]uintptr)(unsafe.Pointer(ole.IID_NULL))

		result, _, _ = syscall.Syscall9(vss.getVTable().addToSnapshotSet, 7,
			uintptr(unsafe.Pointer(vss)), uintptr(unsafe.Pointer(volumeNamePointer)), id[0], id[1],
			id[2], id[3], uintptr(unsafe.Pointer(idSnapshot)), 0, 0)
	} else {
		result, _, _ = syscall.Syscall6(vss.getVTable().addToSnapshotSet, 4,
			uintptr(unsafe.Pointer(vss)), uintptr(unsafe.Pointer(volumeNamePointer)),
			uintptr(unsafe.Pointer(ole.IID_NULL)), uintptr(unsafe.Pointer(idSnapshot)), 0, 0)
	}

	return newVssErrorIfResultNotOK("AddToSnapshotSet() failed", HRESULT(result))
}

// PrepareForBackup calls the equivalent VSS api.
func (vss *IVssBackupComponents) PrepareForBackup() (*IVSSAsync, error) {
	var oleIUnknown *ole.IUnknown
	result, _, _ := syscall.Syscall(vss.getVTable().prepareForBackup, 2,
		uintptr(unsafe.Pointer(vss)), uintptr(unsafe.Pointer(&oleIUnknown)), 0)

	err := newVssErrorIfResultNotOK("PrepareForBackup() failed", HRESULT(result))
	return vss.convertToVSSAsync(oleIUnknown, err)
}

// apiBoolToInt converts a bool for use calling the VSS api
func apiBoolToInt(input bool) uint {
	if input {
		return 1
	}

	return 0
}

// SetBackupState calls the equivalent VSS api.
func (vss *IVssBackupComponents) SetBackupState(selectComponents bool,
	backupBootableSystemState bool, backupType VssBackup, partialFileSupport bool,
) error {
	selectComponentsVal := apiBoolToInt(selectComponents)
	backupBootableSystemStateVal := apiBoolToInt(backupBootableSystemState)
	partialFileSupportVal := apiBoolToInt(partialFileSupport)

	result, _, _ := syscall.Syscall6(vss.getVTable().setBackupState, 5,
		uintptr(unsafe.Pointer(vss)), uintptr(selectComponentsVal),
		uintptr(backupBootableSystemStateVal), uintptr(backupType), uintptr(partialFileSupportVal),
		0)

	return newVssErrorIfResultNotOK("SetBackupState() failed", HRESULT(result))
}

// DoSnapshotSet calls the equivalent VSS api.
func (vss *IVssBackupComponents) DoSnapshotSet() (*IVSSAsync, error) {
	var oleIUnknown *ole.IUnknown
	result, _, _ := syscall.Syscall(vss.getVTable().doSnapshotSet, 2, uintptr(unsafe.Pointer(vss)),
		uintptr(unsafe.Pointer(&oleIUnknown)), 0)

	err := newVssErrorIfResultNotOK("DoSnapshotSet() failed", HRESULT(result))
	return vss.convertToVSSAsync(oleIUnknown, err)
}

// DeleteSnapshots calls the equivalent VSS api.
func (vss *IVssBackupComponents) DeleteSnapshots(snapshotID ole.GUID) (int32, ole.GUID, error) {
	var deletedSnapshots int32 = 0
	var nondeletedSnapshotID ole.GUID
	var result uintptr = 0

	if runtime.GOARCH == "386" {
		id := (*[4]uintptr)(unsafe.Pointer(&snapshotID))

		result, _, _ = syscall.Syscall9(vss.getVTable().deleteSnapshots, 9,
			uintptr(unsafe.Pointer(vss)), id[0], id[1], id[2], id[3],
			uintptr(VSS_OBJECT_SNAPSHOT), uintptr(1), uintptr(unsafe.Pointer(&deletedSnapshots)),
			uintptr(unsafe.Pointer(&nondeletedSnapshotID)),
		)
	} else {
		result, _, _ = syscall.Syscall6(vss.getVTable().deleteSnapshots, 6,
			uintptr(unsafe.Pointer(vss)), uintptr(unsafe.Pointer(&snapshotID)),
			uintptr(VSS_OBJECT_SNAPSHOT), uintptr(1), uintptr(unsafe.Pointer(&deletedSnapshots)),
			uintptr(unsafe.Pointer(&nondeletedSnapshotID)))
	}

	err := newVssErrorIfResultNotOK("DeleteSnapshots() failed", HRESULT(result))
	return deletedSnapshots, nondeletedSnapshotID, err
}

// GetSnapshotProperties calls the equivalent VSS api.
func (vss *IVssBackupComponents) GetSnapshotProperties(snapshotID ole.GUID,
	properties *VssSnapshotProperties) error {
	var result uintptr = 0

	if runtime.GOARCH == "386" {
		id := (*[4]uintptr)(unsafe.Pointer(&snapshotID))

		result, _, _ = syscall.Syscall6(vss.getVTable().getSnapshotProperties, 6,
			uintptr(unsafe.Pointer(vss)), id[0], id[1], id[2], id[3],
			uintptr(unsafe.Pointer(properties)))
	} else {
		result, _, _ = syscall.Syscall(vss.getVTable().getSnapshotProperties, 3,
			uintptr(unsafe.Pointer(vss)), uintptr(unsafe.Pointer(&snapshotID)),
			uintptr(unsafe.Pointer(properties)))
	}

	return newVssErrorIfResultNotOK("GetSnapshotProperties() failed", HRESULT(result))
}

// vssFreeSnapshotProperties calls the equivalent VSS api.
func vssFreeSnapshotProperties(properties *VssSnapshotProperties) error {
	proc, err := findVssProc("VssFreeSnapshotProperties")
	if err != nil {
		return err
	}

	proc.Call(uintptr(unsafe.Pointer(properties)))
	return nil
}

// BackupComplete calls the equivalent VSS api.
func (vss *IVssBackupComponents) BackupComplete() (*IVSSAsync, error) {
	var oleIUnknown *ole.IUnknown
	result, _, _ := syscall.Syscall(vss.getVTable().backupComplete, 2, uintptr(unsafe.Pointer(vss)),
		uintptr(unsafe.Pointer(&oleIUnknown)), 0)

	err := newVssErrorIfResultNotOK("BackupComplete() failed", HRESULT(result))
	return vss.convertToVSSAsync(oleIUnknown, err)
}

// VssSnapshotProperties defines the properties of a VSS snapshot as part of the VSS api.
type VssSnapshotProperties struct {
	snapshotID           ole.GUID
	snapshotSetID        ole.GUID
	snapshotsCount       uint32
	snapshotDeviceObject *uint16
	originalVolumeName   *uint16
	originatingMachine   *uint16
	serviceMachine       *uint16
	exposedName          *uint16
	exposedPath          *uint16
	providerID           ole.GUID
	snapshotAttributes   uint32
	creationTimestamp    uint64
	status               uint
}

// GetSnapshotDeviceObject returns root path to access the snapshot files
// and folders.
func (p *VssSnapshotProperties) GetSnapshotDeviceObject() string {
	return ole.UTF16PtrToString(p.snapshotDeviceObject)
}

// UIID_IVSS_ASYNC defines to GUID of IVSSAsync.
var UIID_IVSS_ASYNC = ole.NewGUID("{507C37B4-CF5B-4e95-B0AF-14EB9767467E}")

// IVSSAsync VSS api interface.
type IVSSAsync struct {
	ole.IUnknown
}

// IVSSAsyncVTable is the vtable for IVSSAsync.
type IVSSAsyncVTable struct {
	ole.IUnknownVtbl
	cancel      uintptr
	wait        uintptr
	queryStatus uintptr
}

// Constants for IVSSAsync api.
const (
	VSS_S_ASYNC_PENDING   = 0x00042309
	VSS_S_ASYNC_FINISHED  = 0x0004230A
	VSS_S_ASYNC_CANCELLED = 0x0004230B
)

// getVTable returns the vtable for IVSSAsync.
func (vssAsync *IVSSAsync) getVTable() *IVSSAsyncVTable {
	return (*IVSSAsyncVTable)(unsafe.Pointer(vssAsync.RawVTable))
}

// Cancel calls the equivalent VSS api.
func (vssAsync *IVSSAsync) Cancel() HRESULT {
	result, _, _ := syscall.Syscall(vssAsync.getVTable().cancel, 1,
		uintptr(unsafe.Pointer(vssAsync)), 0, 0)
	return HRESULT(result)
}

// Wait calls the equivalent VSS api.
func (vssAsync *IVSSAsync) Wait(millis uint32) HRESULT {
	result, _, _ := syscall.Syscall(vssAsync.getVTable().wait, 2, uintptr(unsafe.Pointer(vssAsync)),
		uintptr(millis), 0)
	return HRESULT(result)
}

// QueryStatus calls the equivalent VSS api.
func (vssAsync *IVSSAsync) QueryStatus() (HRESULT, uint32) {
	var state uint32 = 0
	result, _, _ := syscall.Syscall(vssAsync.getVTable().queryStatus, 3,
		uintptr(unsafe.Pointer(vssAsync)), uintptr(unsafe.Pointer(&state)), 0)
	return HRESULT(result), state
}

// WaitUntilAsyncFinished waits until either the async call is finshed or
// the given timeout is reached.
func (vssAsync *IVSSAsync) WaitUntilAsyncFinished(millis uint32) error {
	hresult := vssAsync.Wait(millis)
	err := newVssErrorIfResultNotOK("Wait() failed", hresult)
	if err != nil {
		vssAsync.Cancel()
		return err
	}

	hresult, state := vssAsync.QueryStatus()
	err = newVssErrorIfResultNotOK("QueryStatus() failed", hresult)
	if err != nil {
		vssAsync.Cancel()
		return err
	}

	if state == VSS_S_ASYNC_CANCELLED {
		return newVssTextError("async operation cancelled")
	}

	if state == VSS_S_ASYNC_PENDING {
		vssAsync.Cancel()
		return newVssTextError("async operation pending")
	}

	if state != VSS_S_ASYNC_FINISHED {
		err = newVssErrorIfResultNotOK("async operation failed", HRESULT(state))
		if err != nil {
			return err
		}
	}

	return nil
}

// MountPoint wraps all information of a snapshot of a mountpoint on a volume.
type MountPoint struct {
	isSnapshotted        bool
	snapshotSetID        ole.GUID
	snapshotProperties   VssSnapshotProperties
	snapshotDeviceObject string
}

// IsSnapshotted is true if this mount point was snapshotted successfully.
func (p *MountPoint) IsSnapshotted() bool {
	return p.isSnapshotted
}

// GetSnapshotDeviceObject returns root path to access the snapshot files and folders.
func (p *MountPoint) GetSnapshotDeviceObject() string {
	return p.snapshotDeviceObject
}

// VssSnapshot wraps windows volume shadow copy api (vss) via a simple
// interface to create and delete a vss snapshot.
type VssSnapshot struct {
	iVssBackupComponents *IVssBackupComponents
	snapshotID           ole.GUID
	snapshotProperties   VssSnapshotProperties
	snapshotDeviceObject string
	mountPointInfo       map[string]MountPoint
	timeoutInMillis      uint32
}

// GetSnapshotDeviceObject returns root path to access the snapshot files
// and folders.
func (p *VssSnapshot) GetSnapshotDeviceObject() string {
	return p.snapshotDeviceObject
}

// initializeCOMInterface initialize an instance of the VSS COM api
func initializeVssCOMInterface() (*ole.IUnknown, error) {
	vssInstance, err := loadIVssBackupComponentsConstructor()
	if err != nil {
		return nil, err
	}

	// ensure COM is initialized before use
	ole.CoInitializeEx(0, ole.COINIT_MULTITHREADED)

	var oleIUnknown *ole.IUnknown
	result, _, _ := vssInstance.Call(uintptr(unsafe.Pointer(&oleIUnknown)))
	hresult := HRESULT(result)

	switch hresult {
	case S_OK:
	case E_ACCESSDENIED:
		return oleIUnknown, newVssError(
			"The caller does not have sufficient backup privileges or is not an administrator",
			hresult)
	default:
		return oleIUnknown, newVssError("Failed to create VSS instance", hresult)
	}

	if oleIUnknown == nil {
		return nil, newVssError("Failed to initialize COM interface", hresult)
	}

	return oleIUnknown, nil
}

// HasSufficientPrivilegesForVSS returns nil if the user is allowed to use VSS.
func HasSufficientPrivilegesForVSS() error {
	oleIUnknown, err := initializeVssCOMInterface()
	if oleIUnknown != nil {
		oleIUnknown.Release()
	}

	return err
}

// NewVssSnapshot creates a new vss snapshot. If creating the snapshots doesn't
// finish within the timeout an error is returned.
func NewVssSnapshot(
	volume string, timeoutInSeconds uint, msgError ErrorHandler) (VssSnapshot, error) {
	is64Bit, err := isRunningOn64BitWindows()

	if err != nil {
		return VssSnapshot{}, newVssTextError(fmt.Sprintf(
			"Failed to detect windows architecture: %s", err.Error()))
	}

	if (is64Bit && runtime.GOARCH != "amd64") || (!is64Bit && runtime.GOARCH != "386") {
		return VssSnapshot{}, newVssTextError(fmt.Sprintf("executables compiled for %s can't use "+
			"VSS on other architectures. Please use an executable compiled for your platform.",
			runtime.GOARCH))
	}

	timeoutInMillis := uint32(timeoutInSeconds * 1000)

	oleIUnknown, err := initializeVssCOMInterface()
	if oleIUnknown != nil {
		defer oleIUnknown.Release()
	}
	if err != nil {
		return VssSnapshot{}, err
	}

	comInterface, err := queryInterface(oleIUnknown, UUID_IVSS)
	if err != nil {
		return VssSnapshot{}, err
	}

	/*
		https://microsoft.public.win32.programmer.kernel.narkive.com/aObDj2dD/volume-shadow-copy-backupcomplete-and-vss-e-bad-state

		CreateVSSBackupComponents();
		InitializeForBackup();
		SetBackupState();
		GatherWriterMetadata();
		StartSnapshotSet();
		AddToSnapshotSet();
		PrepareForBackup();
		DoSnapshotSet();
		GetSnapshotProperties();
		<Backup all files>
		VssFreeSnapshotProperties();
		BackupComplete();
	*/

	iVssBackupComponents := (*IVssBackupComponents)(unsafe.Pointer(comInterface))

	if err := iVssBackupComponents.InitializeForBackup(); err != nil {
		iVssBackupComponents.Release()
		return VssSnapshot{}, err
	}

	if err := iVssBackupComponents.SetContext(VSS_CTX_BACKUP); err != nil {
		iVssBackupComponents.Release()
		return VssSnapshot{}, err
	}

	// see https://techcommunity.microsoft.com/t5/Storage-at-Microsoft/What-is-the-difference-between-VSS-Full-Backup-and-VSS-Copy/ba-p/423575

	if err := iVssBackupComponents.SetBackupState(false, false, VSS_BT_COPY, false); err != nil {
		iVssBackupComponents.Release()
		return VssSnapshot{}, err
	}

	err = callAsyncFunctionAndWait(iVssBackupComponents.GatherWriterMetadata,
		"GatherWriterMetadata", timeoutInMillis)
	if err != nil {
		iVssBackupComponents.Release()
		return VssSnapshot{}, err
	}

	if isSupported, err := iVssBackupComponents.IsVolumeSupported(volume); err != nil {
		iVssBackupComponents.Release()
		return VssSnapshot{}, err
	} else if !isSupported {
		iVssBackupComponents.Release()
		return VssSnapshot{}, newVssTextError(fmt.Sprintf("Snapshots are not supported for volume "+
			"%s", volume))
	}

	snapshotSetID, err := iVssBackupComponents.StartSnapshotSet()
	if err != nil {
		iVssBackupComponents.Release()
		return VssSnapshot{}, err
	}

	if err := iVssBackupComponents.AddToSnapshotSet(volume, &snapshotSetID); err != nil {
		iVssBackupComponents.Release()
		return VssSnapshot{}, err
	}

	mountPoints, err := enumerateMountedFolders(volume)
	if err != nil {
		iVssBackupComponents.Release()
		return VssSnapshot{}, newVssTextError(fmt.Sprintf(
			"failed to enumerate mount points for volume %s: %s", volume, err))
	}

	mountPointInfo := make(map[string]MountPoint)

	for _, mountPoint := range mountPoints {
		// ensure every mountpoint is available even without a valid
		// snapshot because we need to consider this when backing up files
		mountPointInfo[mountPoint] = MountPoint{isSnapshotted: false}

		if isSupported, err := iVssBackupComponents.IsVolumeSupported(mountPoint); err != nil {
			continue
		} else if !isSupported {
			continue
		}

		var mountPointSnapshotSetID ole.GUID
		err := iVssBackupComponents.AddToSnapshotSet(mountPoint, &mountPointSnapshotSetID)
		if err != nil {
			iVssBackupComponents.Release()
			return VssSnapshot{}, err
		}

		mountPointInfo[mountPoint] = MountPoint{isSnapshotted: true,
			snapshotSetID: mountPointSnapshotSetID}
	}

	err = callAsyncFunctionAndWait(iVssBackupComponents.PrepareForBackup, "PrepareForBackup",
		timeoutInMillis)
	if err != nil {
		// After calling PrepareForBackup one needs to call AbortBackup() before releasing the VSS
		// instance for proper cleanup.
		// It is not neccessary to call BackupComplete before releasing the VSS instance afterwards.
		iVssBackupComponents.AbortBackup()
		iVssBackupComponents.Release()
		return VssSnapshot{}, err
	}

	err = callAsyncFunctionAndWait(iVssBackupComponents.DoSnapshotSet, "DoSnapshotSet",
		timeoutInMillis)
	if err != nil {
		iVssBackupComponents.AbortBackup()
		iVssBackupComponents.Release()
		return VssSnapshot{}, err
	}

	var snapshotProperties VssSnapshotProperties
	err = iVssBackupComponents.GetSnapshotProperties(snapshotSetID, &snapshotProperties)
	if err != nil {
		iVssBackupComponents.AbortBackup()
		iVssBackupComponents.Release()
		return VssSnapshot{}, err
	}

	for mountPoint, info := range mountPointInfo {

		if !info.isSnapshotted {
			continue
		}

		err := iVssBackupComponents.GetSnapshotProperties(info.snapshotSetID,
			&info.snapshotProperties)
		if err != nil {
			msgError(mountPoint, errors.Errorf(
				"VSS error: GetSnapshotProperties() for mount point %s returned error: ",
				mountPoint, err))
			info.isSnapshotted = false
		} else {
			info.snapshotDeviceObject = info.snapshotProperties.GetSnapshotDeviceObject()
		}

		mountPointInfo[mountPoint] = info
	}

	return VssSnapshot{iVssBackupComponents, snapshotSetID, snapshotProperties,
		snapshotProperties.GetSnapshotDeviceObject(), mountPointInfo, timeoutInMillis}, nil
}

// Delete deletes the created snapshot.
func (p *VssSnapshot) Delete() error {
	var err error
	if err = vssFreeSnapshotProperties(&p.snapshotProperties); err != nil {
		return err
	}

	for _, mountPoint := range p.mountPointInfo {
		if mountPoint.isSnapshotted {
			if err = vssFreeSnapshotProperties(&mountPoint.snapshotProperties); err != nil {
				return err
			}
		}
	}

	if p.iVssBackupComponents != nil {
		defer p.iVssBackupComponents.Release()

		err = callAsyncFunctionAndWait(p.iVssBackupComponents.BackupComplete, "BackupComplete",
			p.timeoutInMillis)
		if err != nil {
			return err
		}

		if _, _, e := p.iVssBackupComponents.DeleteSnapshots(p.snapshotID); e != nil {
			err = newVssTextError(fmt.Sprintf("Failed to delete snapshot: %s", e.Error()))
			p.iVssBackupComponents.AbortBackup()
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// asyncCallFunc is the callback type for callAsyncFunctionAndWait.
type asyncCallFunc func() (*IVSSAsync, error)

// callAsyncFunctionAndWait calls an async functions and waits for it to either
// finish or timeout.
func callAsyncFunctionAndWait(function asyncCallFunc, name string, timeoutInMillis uint32) error {
	iVssAsync, err := function()
	if err != nil {
		return err
	}

	if iVssAsync == nil {
		return newVssTextError(fmt.Sprintf("%s() returned nil", name))
	}

	err = iVssAsync.WaitUntilAsyncFinished(timeoutInMillis)
	iVssAsync.Release()
	return err
}

// loadIVssBackupComponentsConstructor finds the constructor of the VSS api
// inside the VSS dynamic library.
func loadIVssBackupComponentsConstructor() (*windows.LazyProc, error) {
	createInstanceName := "?CreateVssBackupComponents@@YAJPEAPEAVIVssBackupComponents@@@Z"

	if runtime.GOARCH == "386" {
		createInstanceName = "?CreateVssBackupComponents@@YGJPAPAVIVssBackupComponents@@@Z"
	}

	return findVssProc(createInstanceName)
}

// findVssProc find a function with the given name inside the VSS api
// dynamic library.
func findVssProc(procName string) (*windows.LazyProc, error) {
	vssDll := windows.NewLazySystemDLL("VssApi.dll")
	err := vssDll.Load()
	if err != nil {
		return &windows.LazyProc{}, err
	}

	proc := vssDll.NewProc(procName)
	err = proc.Find()
	if err != nil {
		return &windows.LazyProc{}, err
	}

	return proc, nil
}

// queryInterface is a wrapper around the windows QueryInterface api.
func queryInterface(oleIUnknown *ole.IUnknown, guid *ole.GUID) (*interface{}, error) {
	var ivss *interface{}

	result, _, _ := syscall.Syscall(oleIUnknown.VTable().QueryInterface, 3,
		uintptr(unsafe.Pointer(oleIUnknown)), uintptr(unsafe.Pointer(guid)),
		uintptr(unsafe.Pointer(&ivss)))
	if result != 0 {
		return nil, newVssError("QueryInterface failed", HRESULT(result))
	}

	return ivss, nil
}

// isRunningOn64BitWindows returns true if running on 64-bit windows.
func isRunningOn64BitWindows() (bool, error) {
	if runtime.GOARCH == "amd64" {
		return true, nil
	}

	isWow64 := false
	err := windows.IsWow64Process(windows.CurrentProcess(), &isWow64)
	if err != nil {
		return false, err
	}

	return isWow64, nil
}

// enumerateMountedFolders returns all mountpoints on the given volume.
func enumerateMountedFolders(volume string) ([]string, error) {
	var mountedFolders []string

	volumeNamePointer, err := syscall.UTF16PtrFromString(volume)
	if err != nil {
		return mountedFolders, err
	}

	volumeMountPointBuffer := make([]uint16, windows.MAX_LONG_PATH)
	handle, err := windows.FindFirstVolumeMountPoint(volumeNamePointer, &volumeMountPointBuffer[0],
		windows.MAX_LONG_PATH)
	if err != nil {
		// if there are no volumes an error is returned
		return mountedFolders, nil
	}

	defer windows.FindVolumeMountPointClose(handle)

	volumeMountPoint := syscall.UTF16ToString(volumeMountPointBuffer)
	mountedFolders = append(mountedFolders, cleanupVolumeMountPoint(volume, volumeMountPoint))

	for {
		err = windows.FindNextVolumeMountPoint(handle, &volumeMountPointBuffer[0],
			windows.MAX_LONG_PATH)

		if err != nil {
			if err == syscall.ERROR_NO_MORE_FILES {
				break
			}

			return mountedFolders,
				newVssTextError("FindNextVolumeMountPoint() failed: " + err.Error())
		}

		volumeMountPoint := syscall.UTF16ToString(volumeMountPointBuffer)
		mountedFolders = append(mountedFolders, cleanupVolumeMountPoint(volume, volumeMountPoint))
	}

	return mountedFolders, nil
}

func cleanupVolumeMountPoint(volume, mountPoint string) string {
	return strings.ToLower(filepath.Join(volume, mountPoint) + string(filepath.Separator))
}
