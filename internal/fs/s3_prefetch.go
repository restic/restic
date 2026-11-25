package fs

import (
	"path"
	"slices"
	"strings"
	"sync"
)

// s3Prefetcher lists directories in the background before the archiver
// needs them. The archiver walks the tree depth-first and visits the
// subdirectories of a directory in sorted order. For each directory it
// lists, the next few subdirectories it will visit are listed ahead of time,
// so that the latency of these requests overlaps.
type s3Prefetcher struct {
	fs     *s3FS
	window int

	mu sync.Mutex
	// windows contains the sorted subdirectories of the directories the
	// archiver is working on, by path of the directory
	windows map[string]*s3PrefetchWindow
	// results contains the listings which were started in the background
	// and not taken by the archiver yet, by path
	results map[string]*s3Prefetch
}

type s3PrefetchDir struct {
	path   string
	prefix string
}

type s3PrefetchWindow struct {
	dirs []s3PrefetchDir
	// taken is the index of the directory after the one taken last
	taken int
	// next is the index of the next directory to list in the background
	next int
}

type s3Prefetch struct {
	done chan struct{}
	// listing is nil if listing the directory failed
	listing *s3Listing
}

// newS3Prefetcher returns a prefetcher which lists up to window
// subdirectories of each directory ahead, or nil if window is zero.
func newS3Prefetcher(fs *s3FS, window int) *s3Prefetcher {
	if window <= 0 {
		return nil
	}
	return &s3Prefetcher{
		fs:      fs,
		window:  window,
		windows: make(map[string]*s3PrefetchWindow),
		results: make(map[string]*s3Prefetch),
	}
}

// listed is called after the archiver listed dir. It drops the state of
// directories the archiver has finished, and starts listing the first
// subdirectories of dir.
func (p *s3Prefetcher) listed(dir string, subdirs []s3PrefetchDir) {
	if p == nil {
		return
	}
	slices.SortFunc(subdirs, func(a, b s3PrefetchDir) int {
		return strings.Compare(a.path, b.path)
	})

	p.mu.Lock()
	defer p.mu.Unlock()

	// the archiver is working on dir and its parents, all other
	// directories are finished
	for parent := range p.windows {
		if !s3IsParentOrSelf(parent, dir) {
			delete(p.windows, parent)
		}
	}
	for d := range p.results {
		if !s3IsParentOrSelf(path.Dir(d), dir) {
			delete(p.results, d)
		}
	}

	if len(subdirs) == 0 {
		return
	}
	w := &s3PrefetchWindow{dirs: subdirs}
	p.windows[dir] = w
	p.advance(w, 0)
}

// advance starts listing the directories of w from index from on, so that
// up to p.window directories are listed ahead.
func (p *s3Prefetcher) advance(w *s3PrefetchWindow, from int) {
	w.next = max(w.next, from)
	for ; w.next < len(w.dirs) && w.next < from+p.window; w.next++ {
		d := w.dirs[w.next]
		if _, ok := p.results[d.path]; ok {
			continue
		}
		r := &s3Prefetch{done: make(chan struct{})}
		p.results[d.path] = r
		go func() {
			defer close(r.done)
			l, err := p.fs.list(d.path, d.prefix)
			if err == nil {
				r.listing = &l
			}
		}()
	}
}

// take returns the listing of dir if it was started in the background,
// waiting for it if necessary, or nil. It drops the listings of
// subdirectories which the archiver skipped, and starts listing the next
// ones.
func (p *s3Prefetcher) take(dir string) *s3Listing {
	if p == nil {
		return nil
	}

	p.mu.Lock()
	r := p.results[dir]
	delete(p.results, dir)
	if w, ok := p.windows[path.Dir(dir)]; ok {
		i, found := slices.BinarySearchFunc(w.dirs, dir, func(d s3PrefetchDir, target string) int {
			return strings.Compare(d.path, target)
		})
		if found && i >= w.taken {
			for _, d := range w.dirs[w.taken:i] {
				delete(p.results, d.path)
			}
			w.taken = i + 1
			p.advance(w, i+1)
		}
	}
	p.mu.Unlock()

	if r == nil {
		return nil
	}
	<-r.done
	return r.listing
}
