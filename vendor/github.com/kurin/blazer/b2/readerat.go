// Copyright 2017, Google
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package b2

import (
	"io"
	"sync"
)

type readerAt struct {
	rs io.ReadSeeker
	mu sync.Mutex
}

func (r *readerAt) ReadAt(p []byte, off int64) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// ReadAt is supposed to preserve the offset.
	cur, err := r.rs.Seek(0, io.SeekCurrent)
	if err != nil {
		return 0, err
	}
	defer r.rs.Seek(cur, io.SeekStart)

	if _, err := r.rs.Seek(off, io.SeekStart); err != nil {
		return 0, err
	}
	return io.ReadFull(r.rs, p)
}

// wraps a ReadSeeker in a mutex to provite a ReaderAt how is this not in the
// io package?
func enReaderAt(rs io.ReadSeeker) io.ReaderAt {
	return &readerAt{rs: rs}
}
