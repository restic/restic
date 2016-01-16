package naming

// OP defines the corresponding operations for a name resolution change.
type OP uint8

const (
	// No indicates there are no changes.
	No OP = iota
	// Add indicates a new address is added.
	Add
	// Delete indicates an exisiting address is deleted.
	Delete
	// Modify indicates an existing address is modified.
	Modify
)

type ServiceConfig interface{}

// Update defines a name resolution change.
type Update struct {
	// Op indicates the operation of the update.
	Op     OP
	Key    string
	Val    string
	Config ServiceConfig
}

// Resolver does one-shot name resolution and creates a Watcher to
// watch the future updates.
type Resolver interface {
	// Resolve returns the name resolution results.
	Resolve(target string) ([]*Update, error)
	// NewWatcher creates a Watcher to watch the changes on target.
	NewWatcher(target string) Watcher
}

// Watcher watches the updates for a particular target.
type Watcher interface {
	// Next blocks until an update or error happens.
	Next() (*Update, error)
	// Stop stops the Watcher.
	Stop()
}
