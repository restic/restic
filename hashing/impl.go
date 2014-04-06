// Copyright (c) 2014, Alexander Neumann <alexander@bumpern.de>
// All rights reserved.
//
// Redistribution and use in source and binary forms, with or without
// modification, are permitted provided that the following conditions are met:
//
// 1. Redistributions of source code must retain the above copyright notice, this
// list of conditions and the following disclaimer.
//
// 2. Redistributions in binary form must reproduce the above copyright notice,
// this list of conditions and the following disclaimer in the documentation
// and/or other materials provided with the distribution.
//
// THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS" AND
// ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE IMPLIED
// WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE
// DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT HOLDER OR CONTRIBUTORS BE LIABLE
// FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL
// DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR
// SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER
// CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY,
// OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
// OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.

// Packgae hashing provides hashing readers and writers.
package hashing

import (
	"hash"
	"io"
)

// Reader is the interfaces that wraps a normal reader. When Hash() is called,
// it returns the hash for all data that has been read so far.
type Reader interface {
	io.Reader
	Hash() []byte
}

// Writer is the interfaces that wraps a normal writer. When Hash() is called,
// it returns the hash for all data that has been written so far.
type Writer interface {
	io.Writer
	Hash() []byte
}

type reader struct {
	reader io.Reader
	hash   hash.Hash
}

// NewReader wraps an io.Reader and in addition feeds all data read through the
// given hash.
func NewReader(r io.Reader, h func() hash.Hash) *reader {
	return &reader{
		reader: r,
		hash:   h(),
	}
}

func (h *reader) Read(p []byte) (int, error) {
	// call original reader
	n, err := h.reader.Read(p)

	// hash bytes
	if n > 0 {
		// hash
		h.hash.Write(p[0:n])
	}

	// return result
	return n, err
}

func (h *reader) Hash() []byte {
	return h.hash.Sum([]byte{})
}

type writer struct {
	writer io.Writer
	hash   hash.Hash
}

// NewWriter wraps an io.Reader and in addition feeds all data written through
// the given hash.
func NewWriter(w io.Writer, h func() hash.Hash) *writer {
	return &writer{
		writer: w,
		hash:   h(),
	}
}

func (h *writer) Write(p []byte) (int, error) {
	// call original writer
	n, err := h.writer.Write(p)

	// hash bytes
	if n > 0 {
		// hash
		h.hash.Write(p[0:n])
	}

	// return result
	return n, err
}

func (h *writer) Hash() []byte {
	return h.hash.Sum([]byte{})
}
