package gdrive

// TODO make owner of the files configurable (as opposed to the default service account)

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"path"
	"strings"
	"sync"

	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/restic"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/googleapi"
)

//
// fairly generic helpers, likely duplicate code available elsewhere
//

type gdriveFileNotFoundError struct {
	err string
}

func (e gdriveFileNotFoundError) Error() string {
	return e.err
}

// splits provided path into folder path and file name
// unlike path.Split(), dir return value does not have trailing slash
func splitPath(path string) (dir, name string) {
	idx := strings.LastIndexAny(path, "/")
	if idx < 0 {
		return "", path
	}
	return path[:idx], path[idx+1:]
}

//
// low-level google drive access methods
//

const (
	rootFolderID = ""

	folderMimeType = "application/vnd.google-apps.folder"

	folderListPageSize = 128 // artitrary number, api allows 1-1000
)

func gdriveGetItem(ctx context.Context, srv *drive.Service, parentID string, name string) (*drive.File, error) {
	debug.Log("gdriveGetItem(parentID=%q, name=%q)", parentID, name)
	q := "trashed = false"
	q = q + fmt.Sprintf(" and name=\"%s\"", name)
	if parentID != "" {
		q = q + fmt.Sprintf(" and \"%s\" in parents", parentID)
	}
	r, err := srv.Files.List().Q(q).Fields("files(id,name,size,mimeType)").Context(ctx).Do()
	if err != nil {
		return nil, err
	}
	switch len(r.Files) {
	case 0:
		return nil, gdriveFileNotFoundError{fmt.Sprintf("path does not exist (%s)/'%s')", parentID, name)}
	case 1:
		return r.Files[0], nil
	default:
		// restic is not expected to create duplicate files, so either we have a bug or Google
		// created the dup by itself (https://rclone.org/drive/#duplicated-files)
		// if dups prove to be a problem, we'll need automatic disambiguation:
		// * for files, this means picking one that has correct content (i.e. content checksum matches file name)
		// * for folders, this means merging children of all folders with the same name
		return nil, errors.Errorf("ambiguous path name (%s)/'%s')", parentID, name)
	}
}

func gdriveGetFileReader(ctx context.Context, srv *drive.Service, fileID string, length int, offset int64) (io.ReadCloser, error) {
	debug.Log("gdriveGetFileReader(fileID=%q, length=%d, offset=%d)", fileID, length, offset)
	req := srv.Files.Get(fileID)
	isPartial := length > 0 || offset > 0
	if isPartial {
		if length > 0 {
			req.Header().Add("Range", fmt.Sprintf("bytes=%d-%d", offset, offset+int64(length-1)))
		} else {
			req.Header().Add("Range", fmt.Sprintf("bytes=%d-", offset))
		}
	}
	resp, err := req.Context(ctx).Download()
	if err != nil {
		return nil, err
	}
	switch {
	case !isPartial && resp.StatusCode == http.StatusOK:
		return resp.Body, nil
	case resp.StatusCode == http.StatusPartialContent:
		return resp.Body, nil
	case resp.StatusCode == http.StatusOK && resp.ContentLength == int64(length) && offset == 0:
		return resp.Body, nil
	default:
		return nil, errors.Errorf("expected 206 Partial Content, got %s", resp.Status)
	}
}

func gdriveGetFolder(ctx context.Context, srv *drive.Service, parentID string, name string) (*drive.File, error) {
	file, err := gdriveGetItem(ctx, srv, parentID, name)
	if err != nil {
		return nil, err
	}
	if file.MimeType != folderMimeType {
		return nil, errors.Errorf("not a folder (%s)/%s", parentID, name)
	}
	return file, nil
}

func gdriveUploadFile(ctx context.Context, srv *drive.Service, parentID string, name string, rd io.Reader, overwriteIfExists bool) (*drive.File, error) {
	debug.Log("gdriveUploadFile(parentID=%q, name=%q, overwriteIfExists=%t)", parentID, name, overwriteIfExists)
	if parentID == "" || name == "" || rd == nil {
		return nil, errors.New("gdriveCreateFile: parentID, name and rd are required")
	}

	existing, err := gdriveGetItem(ctx, srv, parentID, name)
	if err != nil && !isNotExist(err) {
		return nil, err
	}

	if !overwriteIfExists && existing != nil {
		return nil, errors.Errorf("duplicate file (%s)/%s", parentID, name)
	}

	// most restic uploads appear to be <10MB in size
	//
	// google does not limit simple upload file size, but recommends "For larger files (more than 5 MB)
	// or less reliable network connections, use resumable upload." so we are little over but not by much
	// https://developers.google.com/drive/v3/web/simple-upload
	//
	// even with simple upload, failed uploads appear to be discarded, i.e. no need to deal
	// with partial failed uploads in our code. yay to google.
	//
	// will use simple approach for now and revisit the impl if it proves to cause troubles

	contentType := googleapi.ContentType("binary/octet-stream")

	var file *drive.File
	if existing != nil {
		file, err = srv.Files.Update(existing.Id, nil).Media(rd, contentType).Context(ctx).Do()
	} else {
		file = &drive.File{
			Name:     name,
			Parents:  []string{parentID},
			MimeType: "binary/octet-stream",
		}
		file, err = srv.Files.Create(file).Media(rd, contentType).Context(ctx).Do()
	}

	return file, err
}

// creates new folder
func gdriveCreateFolder(ctx context.Context, srv *drive.Service, parentID string, name string) (*drive.File, error) {
	debug.Log("gdriveCreateFolder(parentID=%q, name=%q)", parentID, name)
	if name == "" {
		return nil, errors.New("gdriveCreateFolder: name is required")
	}
	dir := &drive.File{
		Name:     name,
		MimeType: folderMimeType,
	}
	if parentID != "" {
		dir.Parents = []string{parentID}
	}
	dir, err := srv.Files.Create(dir).Context(ctx).Do()
	if err != nil {
		return nil, err
	}
	return dir, err
}

func gdriveListFolder(ctx context.Context, srv *drive.Service, parentID string, consumer func(*drive.File) error) error {
	debug.Log("gdriveListFolder(parentID=%q)", parentID)
	q := "trashed = false"
	if parentID != "" {
		q += fmt.Sprintf(" and \"%s\" in parents", parentID)
	}
	req := srv.Files.List().Q(q).PageSize(folderListPageSize).Fields("nextPageToken, files(id, name, size)").Context(ctx)
	for {
		resp, err := req.Do()
		if err != nil {
			return err
		}
		for _, file := range resp.Files {
			err = consumer(file)
			if err != nil {
				return err
			}
		}
		if resp.NextPageToken == "" {
			break
		}
		req.PageToken(resp.NextPageToken)
	}
	return nil
}

func gdriveDeleteItem(ctx context.Context, srv *drive.Service, itemID string) error {
	debug.Log("gdriveDeleteItem(itemID=%q)", itemID)
	if itemID == "" {
		return errors.New("gdriveDeleteItem: itemID is required")
	}
	return srv.Files.Delete(itemID).Context(ctx).Do()
}

// TODO should be gdriveNotFound?
func isNotExist(err error) bool {
	if er, ok := err.(*googleapi.Error); ok {
		if er.Code == http.StatusNotFound {
			return true
		}
	}
	if _, ok := err.(gdriveFileNotFoundError); ok {
		return true
	}
	return false
}

//
// gdriveBackend implementation
//

type gdriveBackend struct {
	service *drive.Service

	// lazily populated path->id map, guarded by dirsLock
	dirs     map[string]string
	dirsLock sync.Mutex

	// used to limit number of concurrent remote requests
	sem *backend.Semaphore

	backend.Layout
}

// Ensure that *Backend implements restic.Backend.
var _ restic.Backend = &gdriveBackend{}

func gdriveNewService(jsonKeyPath string, rt http.RoundTripper) (*drive.Service, error) {
	// TODO consider using google's default credentials
	//      https://developers.google.com/accounts/docs/application-default-credentials

	raw, err := ioutil.ReadFile(jsonKeyPath)
	if err != nil {
		return nil, err
	}

	conf, err := google.JWTConfigFromJSON(raw, drive.DriveFileScope, drive.DriveMetadataScope)
	if err != nil {
		return nil, err
	}

	// create authenticating http client using provided http.RoundTripper
	client := conf.Client(context.WithValue(context.Background(), oauth2.HTTPClient, &http.Client{Transport: rt}))

	service, err := drive.New(client)
	if err != nil {
		return nil, err
	}

	return service, nil
}

func getOrCreateFolder(ctx context.Context, service *drive.Service, dirs map[string]string, path string, createMissing bool) (string, error) {
	if dirs[path] != "" {
		return dirs[path], nil // already known
	}
	var err error
	parentID := rootFolderID
	parent, name := splitPath(path)
	if parent != "" {
		parentID, err = getOrCreateFolder(ctx, service, dirs, parent, createMissing)
		if err != nil {
			return "", err
		}
	}
	dir, err := gdriveGetFolder(ctx, service, parentID, name)
	if err != nil && !isNotExist(err) {
		return "", err
	}
	switch {
	case dir != nil:
	case createMissing && isNotExist(err):
		dir, err = gdriveCreateFolder(ctx, service, parentID, name)
		if err != nil {
			return "", err
		}
	default:
		return "", err // ought to be isNotExist error
	}
	dirs[path] = dir.Id
	return dir.Id, nil
}

func open(ctx context.Context, cfg Config, rt http.RoundTripper, createNew bool) (*gdriveBackend, error) {
	debug.Log("open, config %#v, createNew=%t", cfg, createNew)

	service, err := gdriveNewService(cfg.JSONKeyPath, rt)
	if err != nil {
		return nil, err
	}

	layout := &backend.DefaultLayout{Path: cfg.Prefix, Join: path.Join}

	dirs := make(map[string]string)

	if createNew {
		_, err = getOrCreateFolder(ctx, service, dirs, layout.Path, true)
		if err != nil {
			return nil, err
		}

		// make sure config file does not already exist
		confifDir, configFileName := splitPath(layout.Filename(restic.Handle{Type: restic.ConfigFile}))
		configDirID, err := getOrCreateFolder(ctx, service, dirs, confifDir, false)
		if err != nil && !isNotExist(err) {
			return nil, err
		}
		if err == nil {
			_, err = gdriveGetItem(ctx, service, configDirID, configFileName)
			if err == nil {
				return nil, errors.New("config file already exists")
			}
			if err != nil && !isNotExist(err) {
				return nil, err // general error
			}
		}
	}

	sem, err := backend.NewSemaphore(cfg.Connections)
	if err != nil {
		return nil, err
	}

	be := &gdriveBackend{
		Layout:  layout,
		service: service,
		dirs:    dirs,
		sem:     sem,
	}

	return be, nil
}

// Open opens the gdrive backend.
func Open(ctx context.Context, cfg Config, rt http.RoundTripper) (restic.Backend, error) {
	return open(ctx, cfg, rt, false)
}

// Create creates and opens the gdrive backend.
func Create(ctx context.Context, cfg Config, rt http.RoundTripper) (restic.Backend, error) {
	return open(ctx, cfg, rt, true)
}

func (be *gdriveBackend) getFolderID(ctx context.Context, path string) (string, error) {
	be.dirsLock.Lock()
	id, err := getOrCreateFolder(ctx, be.service, be.dirs, path, true)
	be.dirsLock.Unlock()

	if err != nil {
		return "", err
	}
	return id, nil
}

func (be *gdriveBackend) getFile(ctx context.Context, f restic.Handle) (*drive.File, error) {
	parent, name := splitPath(be.Filename(f))
	parentID, err := be.getFolderID(ctx, parent)
	if err != nil {
		return nil, err
	}

	return gdriveGetItem(ctx, be.service, parentID, name)
}

func (be *gdriveBackend) removeItem(ctx context.Context, path string) error {
	parent, name := splitPath(path)
	var parentID string
	var item *drive.File
	var err error

	parentID, err = be.getFolderID(ctx, parent)
	if err == nil {
		item, err = gdriveGetItem(ctx, be.service, parentID, name)
	}

	if err == nil {
		return gdriveDeleteItem(ctx, be.service, item.Id)
	}

	if isNotExist(err) {
		return nil
	}

	return err
}

// Save stores the data in the backend under the given handle.
func (be *gdriveBackend) Save(ctx context.Context, f restic.Handle, rd io.Reader) error {
	parent, name := splitPath(be.Filename(f))
	parentID, err := be.getFolderID(ctx, parent)
	if err != nil {
		return err
	}

	_, err = gdriveUploadFile(ctx, be.service, parentID, name, rd, f.Type == restic.ConfigFile)

	return err
}

// Location returns a string that describes the type and location of the
// repository.
func (be *gdriveBackend) Location() string {
	return be.Layout.(*backend.DefaultLayout).Path
}

// Test a boolean value whether a File with the name and type exists.
func (be *gdriveBackend) Test(ctx context.Context, h restic.Handle) (bool, error) {
	be.sem.GetToken()
	defer be.sem.ReleaseToken()

	_, err := be.getFile(ctx, h)
	if err != nil && be.IsNotExist(err) {
		return false, nil
	}
	return err == nil, err
}

// Remove removes a File described  by h.
func (be *gdriveBackend) Remove(ctx context.Context, f restic.Handle) error {
	be.sem.GetToken()
	defer be.sem.ReleaseToken()

	return be.removeItem(ctx, be.Filename(f))
}

// Close the backend, does nothing
func (be *gdriveBackend) Close() error {
	return nil
}

// Load returns a reader that yields the contents of the file at h at the
// given offset. If length is larger than zero, only a portion of the file
// is returned. rd must be closed after use. If an error is returned, the
// ReadCloser must be nil.
func (be *gdriveBackend) Load(ctx context.Context, h restic.Handle, length int, offset int64) (io.ReadCloser, error) {
	be.sem.GetToken()

	file, err := be.getFile(ctx, h)
	if err != nil {
		be.sem.ReleaseToken()
		return nil, err
	}

	rd, err := gdriveGetFileReader(ctx, be.service, file.Id, length, offset)
	if err != nil {
		be.sem.ReleaseToken()
		return nil, err
	}

	return be.sem.ReleaseTokenOnClose(rd, nil), nil
}

// Stat returns information about the File identified by h.
func (be *gdriveBackend) Stat(ctx context.Context, h restic.Handle) (restic.FileInfo, error) {
	be.sem.GetToken()
	defer be.sem.ReleaseToken()

	file, err := be.getFile(ctx, h)
	if err != nil {
		return restic.FileInfo{}, err
	}
	return restic.FileInfo{Size: file.Size, Name: file.Name}, nil
}

// List returns a channel that yields all names of files of type t in an
// arbitrary order. A goroutine is started for this, which is stopped when
// ctx is cancelled.
func (be *gdriveBackend) List(ctx context.Context, t restic.FileType, fn func(restic.FileInfo) error) error {
	resultForwarder := func(item *drive.File) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			return fn(restic.FileInfo{Name: item.Name, Size: item.Size})
		}
	}

	listChildren := func(path string, consumer func(item *drive.File) error) error {
		id, err := be.getFolderID(ctx, path)
		if err != nil {
			return err
		}

		return gdriveListFolder(ctx, be.service, id, consumer)
	}

	be.sem.GetToken()
	defer be.sem.ReleaseToken()

	prefix, hasSubdirs := be.Basedir(t)

	var err error
	if !hasSubdirs {
		err = listChildren(prefix, resultForwarder)
	} else {
		subdirs := map[string]bool{}
		err = listChildren(prefix, func(item *drive.File) error { subdirs[item.Name] = true; return nil })
		if err == nil {
			for subdir := range subdirs {
				err = listChildren(path.Join(prefix, subdir), resultForwarder)
				if err != nil {
					break
				}
			}
		}
	}
	return err
}

// IsNotExist returns true if the error was caused by a non-existing file
// in the backend.
func (be *gdriveBackend) IsNotExist(err error) bool {
	return isNotExist(err)
}

// Delete removes all data in the backend.
func (be *gdriveBackend) Delete(ctx context.Context) error {
	be.sem.GetToken()
	defer be.sem.ReleaseToken()

	return be.removeItem(ctx, be.Layout.(*backend.DefaultLayout).Path)
}
