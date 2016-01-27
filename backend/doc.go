// Package backend provides local and remote storage for restic repositories.
// All backends need to implement the Backend interface. There is a
// MockBackend, which can be used for mocking in tests, and a MemBackend, which
// stores all data in a hash internally.
package backend
