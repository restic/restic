package fs

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"os"
	"path"
	"strings"
	"syscall"
	"time"

	"github.com/restic/restic/internal/data"
	"github.com/restic/restic/internal/debug"
)

// S3Object is an object or a common prefix returned by an S3 listing. Common
// prefixes have a key ending in "/" and no other fields set.
type S3Object struct {
	Key          string
	Size         int64
	LastModified time.Time
	ETag         string
}

// S3Client provides read access to a single bucket.
type S3Client interface {
	// List calls fn for each object and common prefix directly below prefix,
	// using "/" as the delimiter. If fn returns an error, listing stops and
	// the error is returned.
	List(ctx context.Context, prefix string, fn func(S3Object) error) error
	// Stat returns the object stored at key. If the object does not exist,
	// the returned error wraps os.ErrNotExist.
	Stat(ctx context.Context, key string) (S3Object, error)
	// Get returns the content of the object stored at key and the metadata
	// of the version which is returned. If the object does not exist, the
	// returned error wraps os.ErrNotExist.
	Get(ctx context.Context, key string) (io.ReadCloser, S3Object, error)
}

// S3Options configures the file system returned by NewS3.
type S3Options struct {
	// CacheEntries is the number of directory entries for which metadata
	// from listings is retained. The directory which was used last is kept
	// even if it alone exceeds the limit.
	CacheEntries int
	// PrefetchDirs is the number of subdirectories of each directory which
	// are listed in the background before they are needed.
	PrefetchDirs int
}

const defaultS3CacheEntries = 2 * 1024 * 1024

// s3Epoch is the modification time for directories, which have no metadata
// in S3.
var s3Epoch = time.Unix(0, 0).UTC()

type s3FS struct {
	ctx      context.Context
	client   S3Client
	bucket   string
	cache    *s3DirCache
	prefetch *s3Prefetcher
}

type s3Entry struct {
	fi *ExtendedFileInfo
	// key is the object key for files, or the prefix including the trailing
	// "/" for directories (empty for the bucket itself).
	key string
}

// statically ensure that s3FS implements FS.
var _ FS = &s3FS{}

// NewS3 returns a read-only FS which presents the objects in bucket as a
// directory tree below "/<bucket>". Object keys are split at "/". Keys which
// cannot be represented in a file system tree are skipped, see s3ValidName.
func NewS3(ctx context.Context, client S3Client, bucket string, opts S3Options) (FS, error) {
	if bucket == "" || strings.Contains(bucket, "/") {
		return nil, fmt.Errorf("invalid bucket name %q", bucket)
	}
	limit := opts.CacheEntries
	if limit <= 0 {
		limit = defaultS3CacheEntries
	}
	fs := &s3FS{
		ctx:    ctx,
		client: client,
		bucket: bucket,
		cache:  newS3DirCache(limit),
	}
	fs.prefetch = newS3Prefetcher(fs, opts.PrefetchDirs)
	return fs, nil
}

func s3DirInfo(name string) *ExtendedFileInfo {
	return &ExtendedFileInfo{
		Name:       name,
		Mode:       os.ModeDir | 0755,
		ModTime:    s3Epoch,
		ChangeTime: s3Epoch,
		Links:      1,
	}
}

func s3Compact(obj S3Object) s3CompactEntry {
	// use a hash of the ETag as inode number, this detects modifications
	// which change neither size nor (truncated) modification time
	var inode uint64
	if obj.ETag != "" {
		h := fnv.New64a()
		_, _ = h.Write([]byte(obj.ETag))
		inode = h.Sum64()
	}

	// listings and HEAD requests may return different precisions, so only
	// whole seconds are used
	return s3CompactEntry{
		size:  obj.Size,
		mtime: obj.LastModified.Unix(),
		inode: inode,
	}
}

func (e s3CompactEntry) fileInfo(name string) *ExtendedFileInfo {
	mtime := time.Unix(e.mtime, 0).UTC()
	return &ExtendedFileInfo{
		Name:       name,
		Mode:       0644,
		Size:       e.size,
		ModTime:    mtime,
		ChangeTime: mtime,
		Inode:      e.inode,
		Links:      1,
	}
}

func s3FileInfo(name string, obj S3Object) *ExtendedFileInfo {
	return s3Compact(obj).fileInfo(name)
}

// s3ValidName reports whether a component of an object key can be used as a
// file name. Objects which have the same name as a directory are skipped as
// well.
func s3ValidName(s string) bool {
	return s != "" && s != "." && s != ".."
}

func s3Clean(name string) string {
	return path.Clean("/" + name)
}

// lookup returns the entry for the cleaned absolute path p.
func (fs *s3FS) lookup(p string) (s3Entry, error) {
	if p == "/" {
		return s3Entry{fi: s3DirInfo("/")}, nil
	}
	comps := strings.Split(p[1:], "/")
	if comps[0] != fs.bucket {
		return s3Entry{}, os.ErrNotExist
	}
	if len(comps) == 1 {
		return s3Entry{fi: s3DirInfo(fs.bucket)}, nil
	}

	// the path components are the components of the key
	raw := comps[1:]
	name := raw[len(raw)-1]
	key := strings.Join(raw, "/")

	dir := path.Dir(p)
	for relisted := false; ; relisted = true {
		e, res, wasListed := fs.cache.get(dir, name)
		switch res {
		case s3Found:
			if e.size == s3DirSize {
				return s3Entry{fi: s3DirInfo(name), key: key + "/"}, nil
			}
			return s3Entry{fi: e.fileInfo(name), key: key}, nil
		case s3Missing:
			return s3Entry{}, os.ErrNotExist
		}
		if !wasListed || relisted {
			break
		}

		// The directory was listed before and evicted since, so the
		// archiver is likely still working on it. List it again as a whole
		// instead of requesting each remaining entry individually.
		dirKey := ""
		if len(raw) > 1 {
			dirKey = strings.Join(raw[:len(raw)-1], "/") + "/"
		}
		debug.Log("listing %v again", dir)
		l, err := fs.list(dir, dirKey)
		if err != nil {
			return s3Entry{}, err
		}
		fs.cache.add(dir, l.names, l.entries)
	}

	debug.Log("cache miss for %v, key %q", p, key)

	// a directory takes precedence over an object with the same name
	prefix := key + "/"
	dirExists, err := fs.prefixExists(prefix)
	if err != nil {
		return s3Entry{}, err
	}
	if dirExists {
		return s3Entry{fi: s3DirInfo(name), key: prefix}, nil
	}

	obj, err := fs.client.Stat(fs.ctx, key)
	if err != nil {
		return s3Entry{}, err
	}
	return s3Entry{fi: s3FileInfo(name, obj), key: key}, nil
}

var errS3StopListing = errors.New("stop listing")

func (fs *s3FS) prefixExists(prefix string) (bool, error) {
	found := false
	err := fs.client.List(fs.ctx, prefix, func(S3Object) error {
		found = true
		return errS3StopListing
	})
	if err != nil && !errors.Is(err, errS3StopListing) {
		return false, err
	}
	return found, nil
}

// readdir lists the directory at the cleaned absolute path p and caches the
// metadata of all entries.
func (fs *s3FS) readdir(p string, dir s3Entry) ([]string, error) {
	if p == "/" {
		return []string{fs.bucket}, nil
	}

	l := fs.prefetch.take(p)
	if l == nil {
		listing, err := fs.list(p, dir.key)
		if err != nil {
			return nil, err
		}
		l = &listing
	}
	fs.cache.add(p, l.names, l.entries)
	fs.prefetch.listed(p, l.subdirs)
	return l.names, nil
}

// s3Listing is the result of listing a directory.
type s3Listing struct {
	names   []string
	entries []s3CompactEntry
	subdirs []s3PrefetchDir
}

// list lists the directory at the cleaned absolute path p with the key
// prefix prefix.
func (fs *s3FS) list(p, prefix string) (s3Listing, error) {
	var names []string
	var entries []s3CompactEntry
	dirs := make(map[string]struct{})
	err := fs.client.List(fs.ctx, prefix, func(obj S3Object) error {
		switch {
		case obj.Key == prefix && obj.Size == 0:
			// a zero-sized object for the prefix itself is a directory marker
		case obj.Key != prefix && strings.HasSuffix(obj.Key, "/"):
			dirs[obj.Key[len(prefix):len(obj.Key)-1]] = struct{}{}
		default:
			names = append(names, obj.Key[len(prefix):])
			entries = append(entries, s3Compact(obj))
		}
		return nil
	})
	if err != nil {
		return s3Listing{}, err
	}

	n := 0
	for i, name := range names {
		if _, shadowed := dirs[name]; shadowed || !s3ValidName(name) {
			debug.Log("skipping object %q", prefix+name)
			continue
		}
		names[n], entries[n] = name, entries[i]
		n++
	}
	names, entries = names[:n], entries[:n]
	subdirs := make([]s3PrefetchDir, 0, len(dirs))
	for d := range dirs {
		if !s3ValidName(d) {
			debug.Log("skipping prefix %q", prefix+d+"/")
			continue
		}
		names = append(names, d)
		entries = append(entries, s3CompactEntry{size: s3DirSize})
		subdirs = append(subdirs, s3PrefetchDir{path: path.Join(p, d), prefix: prefix + d + "/"})
	}
	return s3Listing{names: names, entries: entries, subdirs: subdirs}, nil
}

// VolumeName returns leading volume name, for the S3 file system it's
// always the empty string.
func (fs *s3FS) VolumeName(_ string) string {
	return ""
}

func (fs *s3FS) OpenFile(name string, flag int, metadataOnly bool) (File, error) {
	if flag & ^(O_RDONLY|O_NOFOLLOW|O_DIRECTORY) != 0 {
		return nil, pathError("open", name,
			fmt.Errorf("invalid combination of flags 0x%x", flag))
	}

	p := s3Clean(name)
	e, err := fs.lookup(p)
	if err != nil {
		return nil, pathError("open", name, err)
	}
	if flag&O_DIRECTORY != 0 && !e.fi.Mode.IsDir() {
		return nil, pathError("open", name, syscall.ENOTDIR)
	}

	f := &s3File{fs: fs, path: p, entry: e}
	if !metadataOnly {
		if err := f.MakeReadable(); err != nil {
			return nil, err
		}
	}
	return f, nil
}

// Lstat returns the FileInfo structure describing the named file.
// If there is an error, it will be of type *os.PathError.
func (fs *s3FS) Lstat(name string) (*ExtendedFileInfo, error) {
	e, err := fs.lookup(s3Clean(name))
	if err != nil {
		return nil, pathError("lstat", name, err)
	}
	return e.fi, nil
}

// Join joins any number of path elements into a single path.
func (fs *s3FS) Join(elem ...string) string {
	return path.Join(elem...)
}

// Separator returns the OS and FS dependent separator for dirs/subdirs/files.
func (fs *s3FS) Separator() string {
	return "/"
}

// IsAbs reports whether the path is absolute.
func (fs *s3FS) IsAbs(p string) bool {
	return path.IsAbs(p)
}

// Abs returns an absolute representation of path. Relative paths are
// interpreted relative to the root directory.
func (fs *s3FS) Abs(p string) (string, error) {
	return s3Clean(p), nil
}

// Clean returns the cleaned path. For details, see filepath.Clean.
func (fs *s3FS) Clean(p string) string {
	return path.Clean(p)
}

// Base returns the last element of p.
func (fs *s3FS) Base(p string) string {
	return path.Base(p)
}

// Dir returns p without the last element.
func (fs *s3FS) Dir(p string) string {
	return path.Dir(p)
}

type s3File struct {
	fs       *s3FS
	path     string
	entry    s3Entry
	readable bool
	rc       io.ReadCloser
}

// ensure that s3File implements File
var _ File = &s3File{}

// MakeReadable marks the file for reading. The object is requested by the
// first call to ToNode() or Read(). The archiver calls MakeReadable() while
// walking the tree and the others from its concurrent file readers, so
// requesting the object here would serialize all requests.
func (f *s3File) MakeReadable() error {
	if !f.entry.fi.Mode.IsDir() {
		f.readable = true
	}
	return nil
}

// open requests the current version of the object. If the object was
// modified since it was listed, the metadata is replaced by that of the
// version which is read, so that Stat() and ToNode() describe it.
func (f *s3File) open() error {
	if f.rc != nil {
		return nil
	}
	rc, obj, err := f.fs.client.Get(f.fs.ctx, f.entry.key)
	if err != nil {
		return pathError("open", f.path, err)
	}
	f.rc = rc

	e := s3Compact(obj)
	fi := e.fileInfo(f.entry.fi.Name)
	if fi.Size != f.entry.fi.Size || !fi.ModTime.Equal(f.entry.fi.ModTime) || fi.Inode != f.entry.fi.Inode {
		debug.Log("%v was modified since it was listed", f.path)
		f.entry.fi = fi
		f.fs.cache.update(path.Dir(f.path), path.Base(f.path), e)
	}
	return nil
}

func (f *s3File) Read(p []byte) (int, error) {
	if !f.readable {
		return 0, pathError("read", f.path, os.ErrInvalid)
	}
	if err := f.open(); err != nil {
		return 0, err
	}
	return f.rc.Read(p)
}

func (f *s3File) Close() error {
	if f.rc == nil {
		return nil
	}
	err := f.rc.Close()
	f.rc = nil
	return err
}

func (f *s3File) Readdirnames(n int) ([]string, error) {
	if !f.entry.fi.Mode.IsDir() {
		return nil, pathError("readdirnames", f.path, syscall.ENOTDIR)
	}
	if n > 0 {
		return nil, pathError("readdirnames", f.path, errors.New("not implemented"))
	}
	names, err := f.fs.readdir(f.path, f.entry)
	if err != nil {
		return nil, pathError("readdirnames", f.path, err)
	}
	return names, nil
}

func (f *s3File) Stat() (*ExtendedFileInfo, error) {
	return f.entry.fi, nil
}

func (f *s3File) ToNode(_ bool, _ func(format string, args ...any)) (*data.Node, error) {
	if f.readable {
		if err := f.open(); err != nil {
			return nil, err
		}
	}
	node := buildBasicNode(f.path, f.entry.fi)
	node.UID = uint32(os.Getuid())
	node.GID = uint32(os.Getgid())
	node.ChangeTime = f.entry.fi.ChangeTime
	node.Inode = f.entry.fi.Inode
	node.Links = f.entry.fi.Links
	return node, nil
}
