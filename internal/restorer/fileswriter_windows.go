package restorer

import (
	"encoding/json"
	"errors"
	"os"
	"sync"
	"syscall"

	"github.com/restic/restic/internal/fs"
	"github.com/restic/restic/internal/restic"
)

// createOrOpenFile opens the file and handles the readonly attribute and ads related logic during file creation.
// Readonly files - if an existing file is detected as readonly we clear the flag because otherwise we cannot
// make changes to the file. The readonly attribute would be set again in the second pass when the attributes
// are set if the file version being restored has the readonly bit.
// ADS files need special handling - Each stream is treated as a separate file in restic. This method is called
// for the main file which has the streams and for each stream.
// If the ads stream calls this method first and the main file doesn't already exist, then creating the file
// for the streams causes the main file to automatically get created with 0 size. Hence we need to be careful
// while creating the main file. If we blindly create it with the os.O_CREATE option, it could overwrite the
// stream. However creating the stream with os.O_CREATE option does not overwrite the mainfile if it already
// exists. It will simply attach the new stream to the main file if the main file existed, otherwise it will
// create the 0 size main file.
// Another case to handle is if the mainfile already had more streams and the file version being restored has
// less streams, then the extra streams need to be removed from the main file. The stream names are present
// as the value in the generic attribute TypeHasAds.
func createOrOpenFile(path string, createSize int64, fileInfo *fileInfo, allowRecursiveDelete bool) (*os.File, error) {
	if createSize >= 0 {
		var mainPath string
		mainPath, f, err := openFileImpl(path, createSize, fileInfo)
		if err != nil && fs.IsAccessDenied(err) {
			// If file is readonly, clear the readonly flag by resetting the
			// permissions of the file and try again
			// as the metadata will be set again in the second pass and the
			// readonly flag will be applied again if needed.
			if err = fs.ResetPermissions(mainPath); err != nil {
				return nil, err
			}
			if f, err = fs.OpenFile(path, fs.O_WRONLY|fs.O_NOFOLLOW, 0600); err != nil {
				return nil, err
			}
		} else if err != nil && (errors.Is(err, syscall.ELOOP) || errors.Is(err, syscall.EISDIR)) {
			// symlink or directory, try to remove it later on
			f = nil
		} else if err != nil {
			return nil, err
		}
		return postCreateFile(f, path, createSize, allowRecursiveDelete, fileInfo.sparse)
	} else {
		return openFile(path)
	}
}

// openFileImpl is the actual open file implementation.
func openFileImpl(path string, createSize int64, fileInfo *fileInfo) (mainPath string, file *os.File, err error) {
	if createSize >= 0 {
		// File needs to be created or replaced

		//Define all the flags
		var isAlreadyExists bool
		var isAdsRelated, hasAds, isAds = getAdsAttributes(fileInfo.attrs)

		// This means that this is an ads related file. It either has ads streams or is an ads streams

		var mainPath string
		if isAds {
			mainPath = restic.TrimAds(path)
		} else {
			mainPath = path
		}
		if isAdsRelated {
			// Get or create a mutex based on the main file path
			mutex := GetOrCreateMutex(mainPath)
			mutex.Lock()
			defer mutex.Unlock()
			// Making sure the code below doesn't execute concurrently for the main file and any of the ads files
		}

		if err != nil {
			return mainPath, nil, err
		}
		// First check if file already exists
		file, err = openFile(path)
		if err == nil {
			// File already exists
			isAlreadyExists = true
		} else if !os.IsNotExist(err) {
			// Any error other that IsNotExist error, then do not continue.
			// If the error was because access is denied,
			// the calling method will try to check if the file is readonly and if so, it tries to
			// remove the readonly attribute and call this openFileImpl method again once.
			// If this method throws access denied again, then it stops trying and return the error.
			return mainPath, nil, err
		}
		//At this point readonly flag is already handled and we need not consider it anymore.
		file, err = handleCreateFile(path, file, isAdsRelated, hasAds, isAds, isAlreadyExists)
	} else {
		// File is already created. For subsequent writes, only use os.O_WRONLY flag.
		file, err = openFile(path)
	}

	return mainPath, file, err
}

// handleCreateFile handles all the various combination of states while creating the file if needed.
func handleCreateFile(path string, fileIn *os.File, isAdsRelated, hasAds, isAds, isAlreadyExists bool) (file *os.File, err error) {
	if !isAdsRelated {
		// This is the simplest case where ADS files are not involved.
		file, err = handleCreateFileNonAds(path, fileIn, isAlreadyExists)
	} else {
		// This is a complex case needing coordination between the main file and the ads files.
		file, err = handleCreateFileAds(path, fileIn, hasAds, isAds, isAlreadyExists)
	}

	return file, err
}

// handleCreateFileNonAds handles all the various combination of states while creating the non-ads file if needed.
func handleCreateFileNonAds(path string, fileIn *os.File, isAlreadyExists bool) (file *os.File, err error) {
	// This is the simple case.
	if isAlreadyExists {
		// If the non-ads file already exists, return the file
		// that we already created without create option.
		return fileIn, nil
	} else {
		// If the non-ads file did not exist, try creating the file with create flag.
		return fs.OpenFile(path, fs.O_CREATE|fs.O_WRONLY|fs.O_NOFOLLOW, 0600)
	}
}

// handleCreateFileAds handles all the various combination of states while creating the ads related file if needed.
func handleCreateFileAds(path string, fileIn *os.File, hasAds, isAds, isAlreadyExists bool) (file *os.File, err error) {
	// This is the simple case. We do not need to change the encryption attribute.
	if isAlreadyExists {
		// If the ads related file already exists and no change to encryption, return the file
		// that we already created without create option.
		return fileIn, nil
	} else {
		// If the ads related file did not exist, first check if it is a hasAds or isAds
		if isAds {
			// If it is an ads file, then we can simple open it with create options without worrying about overwriting.
			return fs.OpenFile(path, fs.O_CREATE|fs.O_WRONLY|fs.O_NOFOLLOW, 0600)
		}
		if hasAds {
			// If it is the main file which has ads files attached, we will check again if the main file wasn't created
			// since we synced.
			file, err = openFile(path)
			if err != nil {
				if os.IsNotExist(err) {
					// We confirmed that the main file still doesn't exist after syncing.
					// Hence creating the file with the create flag.
					// Directly open the main file with create option as it should not be encrypted.
					return fs.OpenFile(path, fs.O_CREATE|fs.O_WRONLY|fs.O_NOFOLLOW, 0600)
				} else {
					// Some other error occured so stop processing and return it.
					return nil, err
				}
			} else {
				// This means that the main file exists now and requires no change to encryption. Simply return it.
				return file, err
			}
		}
		return nil, errors.New("invalid case for ads same file encryption")
	}
}

// Helper methods

var pathMutexMap = PathMutexMap{
	mutex: make(map[string]*sync.Mutex),
}

// PathMutexMap represents a map of mutexes, where each path maps to a unique mutex.
type PathMutexMap struct {
	mu    sync.RWMutex
	mutex map[string]*sync.Mutex
}

// CleanupPath performs clean up for the specified path.
func CleanupPath(path string) {
	removeMutex(path)
}

// removeMutex removes the mutex for the specified path.
func removeMutex(path string) {
	path = restic.TrimAds(path)
	pathMutexMap.mu.Lock()
	defer pathMutexMap.mu.Unlock()

	// Delete the mutex from the map
	delete(pathMutexMap.mutex, path)
}

// Cleanup performs cleanup for all paths.
// It clears all the mutexes in the map.
func Cleanup() {
	pathMutexMap.mu.Lock()
	defer pathMutexMap.mu.Unlock()
	// Iterate over the map and remove each mutex
	for path, mutex := range pathMutexMap.mutex {
		// You can optionally do additional cleanup or release resources associated with the mutex
		mutex.Lock()
		// Delete the mutex from the map
		delete(pathMutexMap.mutex, path)
		mutex.Unlock()
	}
}

// GetOrCreateMutex returns the mutex associated with the given path.
// If the mutex doesn't exist, it creates a new one.
func GetOrCreateMutex(path string) *sync.Mutex {
	pathMutexMap.mu.RLock()
	mutex, ok := pathMutexMap.mutex[path]
	pathMutexMap.mu.RUnlock()

	if !ok {
		// The mutex doesn't exist, upgrade the lock and create a new one
		pathMutexMap.mu.Lock()
		defer pathMutexMap.mu.Unlock()

		// Double-check if another goroutine has created the mutex
		if mutex, ok = pathMutexMap.mutex[path]; !ok {
			mutex = &sync.Mutex{}
			pathMutexMap.mutex[path] = mutex
		}
	}
	return mutex
}

// getAdsAttributes gets all the ads related attributes.
func getAdsAttributes(attrs map[restic.GenericAttributeType]json.RawMessage) (isAdsRelated, hasAds, isAds bool) {
	if len(attrs) > 0 {
		adsBytes := attrs[restic.TypeHasADS]
		hasAds = adsBytes != nil
		isAds = string(attrs[restic.TypeIsADS]) != "true"
	}
	isAdsRelated = hasAds || isAds
	return isAdsRelated, hasAds, isAds
}
