// Package restorer contains code to restore data from a repository.
//
// The Restorer tries to keep the number of backend requests minimal. It does
// this by downloading all required blobs of a pack file with a single backend
// request and avoiding repeated downloads of the same pack. In addition,
// several pack files are fetched concurrently.
//
// Here is high-level pseudo-code of how the Restorer attempts to achieve
// these goals:
//
//	while there are packs to process
//	  choose a pack to process                      [1]
//	  retrieve the pack from the backend            [2]
//	  write pack blobs to the files that need them  [3]
//
// Retrieval of repository packs (step [2]) and writing target files (step [3])
// are performed concurrently on multiple goroutines.
//
// Implementation does not guarantee order in which blobs are written to the
// target files and, for example, the last blob of a file can be written to the
// file before any of the preceding file blobs. It is therefore possible to
// have gaps in the data written to the target files if restore fails or
// interrupted by the user.
package restorer
