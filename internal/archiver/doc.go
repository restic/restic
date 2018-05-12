// Package archiver contains the code which reads files, splits them into
// chunks and saves the data to the repository.
//
// An Archiver has a number of worker goroutines handling saving the different
// data structures to the repository, the details are implemented by the
// FileSaver, BlobSaver, and TreeSaver types.
//
// The main goroutine (the one calling Snapshot()) traverses the directory tree
// and delegates all work to these worker pools. They return a type
// (FutureFile, FutureBlob, and FutureTree) which can be resolved later, by
// calling Wait() on it.
package archiver
