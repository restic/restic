// Package repository implements a restic repository on top of a backend. In
// the following the abstractions used for this package are listed. More
// information can be found in the restic design document.
//
// # File
//
// A file is a named handle for some data saved in the backend. For the local
// backend, this corresponds to actual files saved to disk. Usually, the SHA256
// hash of the content is used for a file's name (hexadecimal, in lower-case
// ASCII characters). An exception is the file `config`. Most files are
// encrypted before being saved in a backend. This means that the name is the
// hash of the ciphertext.
//
// # Blob
//
// A blob is a number of bytes that has a type (data or tree). Blobs are
// identified by an ID, which is the SHA256 hash of the blobs' contents. One or
// more blobs are bundled together in a Pack and then saved to the backend.
// Blobs are always encrypted before being bundled in a Pack.
//
// # Pack
//
// A Pack is a File in the backend that contains one or more (encrypted) blobs,
// followed by a header at the end of the Pack. The header is encrypted and
// contains the ID, type, length and offset for each blob contained in the
// Pack.
package repository
