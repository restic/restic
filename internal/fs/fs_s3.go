package fs

import (
	"context"
	"fmt"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/restic/restic/internal/data"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path"
	"strings"
	"sync"
	"time"
)

const S3Prefix = "s3:/"
const basePermissionFile fs.FileMode = 0644
const basePermissionFolder fs.FileMode = os.ModeDir | 0755

type S3Source struct {
	s3Client      *minio.Client
	files         map[string]*ExtendedFileInfo
	filesByFolder map[string][]string
}

// statically ensure that S3Source implements FS.
var _ FS = &S3Source{}

func (fs *S3Source) VolumeName(_ string) string {
	return ""
}

// OpenFile opens a file or directory for reading.
func (fs *S3Source) OpenFile(name string, _ int, metadataOnly bool) (File, error) {
	name = s3CleanPath(name)
	if name == "/" {
		return nil, fmt.Errorf("invalid filename specified")
	}

	fi, ok := fs.files[name]
	if !ok {
		return nil, pathError("open file", name, os.ErrNotExist)
	}

	return newS3SourceFile(name, fi, fs.s3Client,
		// is not folder, value is nil
		fs.filesByFolder[name], metadataOnly)
}

func (fs *S3Source) factoryS3Client() (*minio.Client, error) {
	endpoint := os.Getenv("AWS_ENDPOINT_URL")
	accessKeyID := os.Getenv("AWS_ACCESS_KEY_ID")
	secretAccessKey := os.Getenv("AWS_SECRET_ACCESS_KEY")
	if accessKeyID == "" && secretAccessKey != "" {
		return nil, errors.Fatalf("no credentials found. $AWS_SECRET_ACCESS_KEY is set but $AWS_ACCESS_KEY_ID is empty")
	} else if accessKeyID != "" && secretAccessKey == "" {
		return nil, errors.Fatalf("no credentials found. $AWS_ACCESS_KEY_ID is set but $AWS_SECRET_ACCESS_KEY is empty")
	} else if endpoint == "" {
		return nil, errors.Fatalf("no credentials found. $AWS_ENDPOINT_URL is empty")
	}

	urlEndpoint, err := url.Parse(endpoint)
	if err != nil {
		return nil, err
	}
	s3Client, err := minio.New(urlEndpoint.Host, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKeyID, secretAccessKey, ""),
		Secure: urlEndpoint.Scheme == "https",
	})
	if err != nil {
		return nil, err
	}

	return s3Client, nil
}

func (fs *S3Source) WarmingUp(targets []string) error {
	stateDate := time.Now()
	defer func() {
		debug.Log("s3 duration warming up %s", time.Since(stateDate))
	}()

	var err error
	fs.s3Client, err = fs.factoryS3Client()

	if err != nil {
		return err
	}

	var muFilesByFolder sync.Mutex
	filesByFolder := make(map[string][]string)
	var muFiles sync.Mutex
	files := make(map[string]*ExtendedFileInfo)

	var wg sync.WaitGroup
	wg.Add(len(targets))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, len(targets))
	for _, target := range targets {
		partPath := strings.Split(target, "/")
		// example /bucket-name
		bucketName := partPath[1]
		prefix := path.Join(partPath[2:]...)
		root := path.Join("/", bucketName)

		go func() {
			defer wg.Done()
			for obj := range fs.s3Client.ListObjects(ctx, bucketName, minio.ListObjectsOptions{Recursive: true, Prefix: prefix}) {
				if obj.Err != nil {
					if ctx.Err() == nil {
						select {
						case errCh <- obj.Err:
						default:
						}
					}
					cancel()
					return
				}

				absPath := path.Join(root, obj.Key)
				for currPath := absPath; ; {
					currPath = path.Clean(path.Dir(currPath))
					if currPath == "/" {
						break
					}

					muFiles.Lock()
					if _, exists := files[currPath]; exists {
						muFiles.Unlock()
						// this tree already added
						break
					}
					files[currPath] = &ExtendedFileInfo{
						Name:       path.Base(currPath),
						Mode:       basePermissionFolder,
						ModTime:    time.Unix(0, 0),
						ChangeTime: time.Unix(0, 0),
						Size:       0,
					}
					muFiles.Unlock()
				}
				{
					dir, file := path.Split(absPath)
					dir = path.Clean(dir)
					muFilesByFolder.Lock()
					filesByFolder[dir] = append(filesByFolder[dir], file)
					muFilesByFolder.Unlock()
				}

				muFiles.Lock()
				files[absPath] = &ExtendedFileInfo{
					Name:       path.Base(absPath),
					Mode:       basePermissionFile,
					ModTime:    obj.LastModified,
					ChangeTime: obj.LastModified,
					Size:       obj.Size,
				}
				muFiles.Unlock()
			}
		}()
	}
	wg.Wait()
	close(errCh)

	select {
	case err, ok := <-errCh:
		if err != nil && ok {
			return err
		}
	default:
	}

	fs.filesByFolder = filesByFolder
	fs.files = files
	return nil
}

// Lstat returns the FileInfo structure describing the named file.
// If there is an error, it will be of type *os.PathError.
func (fs *S3Source) Lstat(name string) (*ExtendedFileInfo, error) {
	name = s3CleanPath(name)
	info, ok := fs.files[name]
	if !ok {
		return nil, pathError("lstat", name, os.ErrNotExist)
	}
	return info, nil
}

func (fs *S3Source) Join(elem ...string) string {
	return path.Join(elem...)
}

func (fs *S3Source) Separator() string {
	return "/"
}

func (fs *S3Source) IsAbs(p string) bool {
	return path.IsAbs(p)
}
func (fs *S3Source) Abs(p string) (string, error) {
	return s3CleanPath(p), nil
}

func s3CleanPath(name string) string {
	return path.Clean("/" + name)
}

func (fs *S3Source) Clean(p string) string {
	return path.Clean(p)
}

func (fs *S3Source) Base(p string) string {
	return path.Base(p)
}

func (fs *S3Source) Dir(p string) string {
	return path.Dir(p)
}

type s3SourceFile struct {
	rc            io.ReadCloser
	name          string
	fi            *ExtendedFileInfo
	filesInFolder []string
	s3Client      *minio.Client
}

// See the File interface for a description of each method
var _ File = &s3SourceFile{}

func newS3SourceFile(name string, fi *ExtendedFileInfo, s3Client *minio.Client, filesInFolder []string, metadataOnly bool) (*s3SourceFile, error) {
	name = s3CleanPath(name)
	if metadataOnly || fi.Mode.IsDir() {
		return &s3SourceFile{name: name, fi: fi, rc: nil, filesInFolder: filesInFolder, s3Client: s3Client}, nil
	}

	partPath := strings.Split(name, "/")
	// example /bucket-name
	bucketName := partPath[1]
	objPath := path.Join(partPath[2:]...)
	ctx := context.Background()
	object, err := s3Client.GetObject(ctx, bucketName, objPath, minio.GetObjectOptions{})
	if err != nil {
		return nil, pathError("open file s3", name, os.ErrNotExist)
	}
	return &s3SourceFile{name: name, fi: fi, rc: object, filesInFolder: filesInFolder, s3Client: s3Client}, nil

}

func (f *s3SourceFile) MakeReadable() error {
	if f.rc != nil {
		panic("s3 file is already readable")
	}

	newF, err := newS3SourceFile(f.name, f.fi, f.s3Client, f.filesInFolder, false)
	if err != nil {
		return err
	}
	// replace state and also reset cached FileInfo
	*f = *newF
	return nil
}

func (f *s3SourceFile) Stat() (*ExtendedFileInfo, error) {
	return f.fi, nil
}

func (f *s3SourceFile) ToNode(_ bool, _ func(format string, args ...any)) (*data.Node, error) {
	node := buildBasicNode(f.name, f.fi)

	//TODO: change on info about owner in repo
	node.UID = 0 //uint32(os.Getuid())
	node.GID = 0 //uint32(os.Getgid())
	node.ChangeTime = node.ModTime

	return node, nil
}

func (f *s3SourceFile) Read(p []byte) (n int, err error) {
	if f.rc != nil {
		return f.rc.Read(p)
	}

	return 0, pathError("read", f.name, os.ErrNotExist)
}

func (f *s3SourceFile) Readdirnames(_ int) ([]string, error) {
	if f.filesInFolder == nil {
		return []string{}, pathError("Readdirnames", f.name, os.ErrNotExist)
	}
	return f.filesInFolder, nil
}

func (f *s3SourceFile) Close() error {
	if f.rc != nil {
		return f.rc.Close()
	}
	return nil
}
