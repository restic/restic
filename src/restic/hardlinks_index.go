package restic

import (
	"sync"
	"fmt"
)

type hardlinkKey struct {
    Inode, Device uint64
}

type hardlinkData struct {
    Links uint64
    Name string
}

var (
	hardLinkIndex      = make(map[hardlinkKey]*hardlinkData)
	hardLinkIndexMutex = sync.RWMutex{}
)

// ExistsLink checks wether the link already exist in the index
func ExistsLink(inode uint64, device uint64) bool {
	hardLinkIndexMutex.RLock()
	defer hardLinkIndexMutex.RUnlock()
	_, ok := hardLinkIndex[hardlinkKey{inode, device}]
	
	return ok
}

// Addlinks adds a link to the index
func AddLink(inode uint64, device uint64, links uint64, name string) {
	hardLinkIndexMutex.RLock()
	_, ok := hardLinkIndex[hardlinkKey{inode, device}]
	hardLinkIndexMutex.RUnlock()
	
	if !ok {
		hardLinkIndexMutex.Lock()
		hardLinkIndex[hardlinkKey{inode,device}] = &hardlinkData{links, name};
		hardLinkIndexMutex.Unlock()
	} 	
}

// Getlink obtains a link from the index
func GetLink(inode uint64, device uint64) *hardlinkData {
	hardLinkIndexMutex.RLock()
	defer hardLinkIndexMutex.RUnlock()
	return hardLinkIndex[hardlinkKey{inode, device}]
}

// RemoveLink removes a link from the index
func RemoveLink(inode uint64, device uint64) {
	hardLinkIndexMutex.Lock()
	defer hardLinkIndexMutex.Unlock()
	delete(hardLinkIndex, hardlinkKey{inode, device})
}

// DecrementLinks decrements the count of a link in the index
func DecrementLink(inode uint64, device uint64) {
	hardLinkIndexMutex.RLock()
	_, ok := hardLinkIndex[hardlinkKey{inode, device}]
	hardLinkIndexMutex.RUnlock()
	
	if ok {
		hardLinkIndexMutex.RLock()
		if hardLinkIndex[hardlinkKey{inode, device}].Links > 0 {
			hardLinkIndexMutex.RUnlock()
			hardLinkIndexMutex.Lock()
			hardLinkIndex[hardlinkKey{inode, device}].Links--
			hardLinkIndexMutex.Unlock()
		} else {
			hardLinkIndexMutex.RUnlock()
		}
	}
}

// CountLink return the number of links for a given inode-device combination
func CountLink(inode uint64, device uint64) uint64 {
	hardLinkIndexMutex.RLock()
	defer hardLinkIndexMutex.RUnlock()
	
	_, ok := hardLinkIndex[hardlinkKey{inode, device}]
	if ok {
		return hardLinkIndex[hardlinkKey{inode, device}].Links
	}
	return 0
}

// PrintLinkSummary prints the unresolved links during the restore process
func PrintLinkSummary() {
	hardLinkIndexMutex.RLock()
	defer hardLinkIndexMutex.RUnlock()
	
	for _, value := range hardLinkIndex {
	    fmt.Printf("File %v has unrestored links; probably you did not backup the complete volume\n", value.Name)
	}
}
