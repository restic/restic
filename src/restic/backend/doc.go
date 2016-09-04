// Package backend provides local and remote storage for restic repositories.
// All backends need to implement the Backend interface. There is a MemBackend,
// which stores all data in a map internally and can be used for testing.
package backend
