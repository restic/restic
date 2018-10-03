// Package restic gives a (very brief) introduction to the structure of source code.
//
// Overview
//
// The packages are structured so that cmd/ contains the main package for the
// restic binary, and internal/ contains almost all code in library form. We've
// chosen to use the internal/ path so that the packages cannot be imported by
// other programs. This was done on purpose, at the moment restic is a
// command-line program and not a library. This may be revisited at a later
// point in time.
package restic
