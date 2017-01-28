package restic

import (
	"sync"
	"fmt"
)

type HardlinkKey struct {
    Inode, Device uint64
}

type HardlinkData struct {
    Links uint64
    Name string
}

var (
	hardLinkIndex      = make(map[HardlinkKey]*HardlinkData)
	hardLinkIndexMutex = sync.RWMutex{}
)

func ExistsLink(inode uint64, device uint64) bool {
	hardLinkIndexMutex.RLock()
	defer hardLinkIndexMutex.RUnlock()
	_, ok := hardLinkIndex[HardlinkKey{inode, device}]
	
	return ok
}

func AddLink(inode uint64, device uint64, links uint64, name string) {
	hardLinkIndexMutex.RLock()
	_, ok := hardLinkIndex[HardlinkKey{inode, device}]
	hardLinkIndexMutex.RUnlock()
	
	if !ok {
		hardLinkIndexMutex.Lock()
		hardLinkIndex[HardlinkKey{inode,device}] = &HardlinkData{links, name};
		hardLinkIndexMutex.Unlock()
	} 	
}

func GetLink(inode uint64, device uint64) *HardlinkData {
	hardLinkIndexMutex.RLock()
	defer hardLinkIndexMutex.RUnlock()
	return hardLinkIndex[HardlinkKey{inode, device}]
}

func RemoveLink(inode uint64, device uint64) {
	hardLinkIndexMutex.Lock()
	defer hardLinkIndexMutex.Unlock()
	delete(hardLinkIndex, HardlinkKey{inode, device})
}

func DecrementLink(inode uint64, device uint64) {
	hardLinkIndexMutex.RLock()
	_, ok := hardLinkIndex[HardlinkKey{inode, device}]
	hardLinkIndexMutex.RUnlock()
	
	if ok {
		hardLinkIndexMutex.RLock()
		if hardLinkIndex[HardlinkKey{inode, device}].Links > 0 {
			hardLinkIndexMutex.RUnlock()
			hardLinkIndexMutex.Lock()
			hardLinkIndex[HardlinkKey{inode, device}].Links--
			hardLinkIndexMutex.Unlock()
		} else {
			hardLinkIndexMutex.RUnlock()
		}
	}
}

/* return the number of links for a given inode-device combination */
func CountLink(inode uint64, device uint64) uint64 {
	hardLinkIndexMutex.RLock()
	defer hardLinkIndexMutex.RUnlock()
	
	_, ok := hardLinkIndex[HardlinkKey{inode, device}]
	if ok {
		return hardLinkIndex[HardlinkKey{inode, device}].Links
	}
	return 0
}

func PrintLinkSummary() {
	hardLinkIndexMutex.RLock()
	defer hardLinkIndexMutex.RUnlock()
	
	for _, value := range hardLinkIndex {
	    fmt.Printf("File %v has unrestored links; probably you did not backup the complete volume\n", value.Name)
	}
}
