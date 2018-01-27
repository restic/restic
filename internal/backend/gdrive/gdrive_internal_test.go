package gdrive

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"sort"
	"strings"
	"testing"
	"time"

	rtest "github.com/restic/restic/internal/test"
	drive "google.golang.org/api/drive/v3"
)

func assertEmptyString(t *testing.T, s string) {
	if s != "" {
		t.Fatalf("%q is not empty", s)
	}
}

func TestSplitPath(t *testing.T) {
	var dir, name string

	dir, name = splitPath("a")
	assertEmptyString(t, dir)
	rtest.Equals(t, "a", name)

	dir, name = splitPath("a/b")
	rtest.Equals(t, "a", dir)
	rtest.Equals(t, "b", name)

	dir, name = splitPath("a/b/c")
	rtest.Equals(t, "a/b", dir)
	rtest.Equals(t, "c", name)

	dir, name = splitPath("a/b/")
}

func getTestPrefix(t *testing.T) string {
	prefix := os.Getenv("RESTIC_TEST_GDRIVE_PREFIX")
	if prefix == "" {
		t.Skipf("$RESTIC_TEST_GDRIVE_PREFIX is not set")
	}

	return fmt.Sprintf("%s/test-%d", prefix, time.Now().UnixNano())
}
func getJSONKeyPath(t *testing.T) string {
	jsonKeyPath := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")
	if jsonKeyPath == "" {
		t.Skipf("$GOOGLE_APPLICATION_CREDENTIALS is not set")
	}

	return jsonKeyPath
}

func setupTest(t *testing.T) (*drive.Service, string) {
	jsonKeyPath := getJSONKeyPath(t)
	prefix := getTestPrefix(t)

	service, err := gdriveNewService(jsonKeyPath, http.DefaultTransport)
	rtest.OK(t, err)

	return service, prefix
}

func TestGetOrCreateFolder(t *testing.T) {
	service, prefix := setupTest(t)

	dirs := make(map[string]string)

	folderID, err := getOrCreateFolder(context.TODO(), service, dirs, prefix, true)
	rtest.OK(t, err)
	if folderID == "" {
		t.Fatalf("could not get or create folder")
	}
	defer gdriveDeleteItem(context.TODO(), service, folderID)
	if dirs[prefix] != folderID {
		t.Fatalf("invalid dirs map")
	}

	folderID, err = getOrCreateFolder(context.TODO(), service, dirs, prefix+"/a/b/c/d", true)
	rtest.OK(t, err)
	if folderID == "" {
		t.Fatalf("could not get or create folder")
	}
	if dirs[prefix+"/a/b/c/d"] != folderID {
		t.Fatalf("invalid dirs map")
	}
}

func assertFolderList(t *testing.T, service *drive.Service, folderID string, expected ...string) {
	var actual []string
	capture := func(file *drive.File) error {
		actual = append(actual, file.Name)
		return nil
	}

	rtest.OK(t, gdriveListFolder(context.TODO(), service, folderID, capture))

	sort.Strings(expected)
	sort.Strings(actual)

	rtest.Equals(t, expected, actual)
}

func TestFolderList(t *testing.T) {
	service, prefix := setupTest(t)

	folder, err := gdriveCreateFolder(context.TODO(), service, rootFolderID, prefix)
	rtest.OK(t, err)
	defer gdriveDeleteItem(context.TODO(), service, folder.Id)

	gdriveUploadFile(context.TODO(), service, folder.Id, "file-1", strings.NewReader("file-1"), true)
	file2, _ := gdriveUploadFile(context.TODO(), service, folder.Id, "file-2", strings.NewReader("file-2"), true)
	assertFolderList(t, service, folder.Id, "file-1", "file-2")

	gdriveDeleteItem(context.TODO(), service, file2.Id)
	assertFolderList(t, service, folder.Id, "file-1") // trashed==true
}

func TestFolderList_pagination(t *testing.T) {
	if os.Getenv("RESTIC_TEST_GDRIVE_SLOW") == "" {
		t.Skipf("$RESTIC_TEST_GDRIVE_SLOW is not set")
	}

	service, prefix := setupTest(t)

	folder, err := gdriveCreateFolder(context.TODO(), service, rootFolderID, prefix)
	rtest.OK(t, err)
	defer gdriveDeleteItem(context.TODO(), service, folder.Id)

	// it takes several minutes to create test files, be patient
	names := []string{}
	for i := 1; i < folderListPageSize+10; i++ {
		name := fmt.Sprintf("file-%d", i)
		_, err = gdriveUploadFile(context.TODO(), service, folder.Id, name, strings.NewReader(name), true)
		rtest.OK(t, err)
		names = append(names, name)
	}
	assertFolderList(t, service, folder.Id, names...)
}

func TestFileUpload(t *testing.T) {
	service, prefix := setupTest(t)

	folder, err := gdriveCreateFolder(context.TODO(), service, rootFolderID, prefix)
	rtest.OK(t, err)
	defer gdriveDeleteItem(context.TODO(), service, folder.Id)

	assertUpload := func(name string, size int) {
		data := rtest.Random(0, size)
		_, err = gdriveUploadFile(context.TODO(), service, folder.Id, name, bytes.NewReader(data), true)
		rtest.OK(t, err)

		file, err := gdriveGetItem(context.TODO(), service, folder.Id, name)
		rtest.OK(t, err)
		rtest.Equals(t, int64(size), file.Size)

		rd, err := gdriveGetFileReader(context.TODO(), service, file.Id, 0, 0)
		rtest.OK(t, err)
		defer rd.Close()

		echo, err := ioutil.ReadAll(rd)
		rtest.OK(t, err)
		rtest.Equals(t, data, echo)
	}

	// upload new
	assertUpload("small", 100)

	// update existing
	assertUpload("small", 200)
	file, err := gdriveGetItem(context.TODO(), service, folder.Id, "small")
	rtest.OK(t, err)
	rtest.Equals(t, int64(200), file.Size)

	// upload larger file
	assertUpload("large", 10*1024*1024+100)
}

func TestRemoveItem(t *testing.T) {
	cfg := NewConfig()
	cfg.JSONKeyPath = getJSONKeyPath(t)
	cfg.Prefix = getTestPrefix(t)
	be, err := open(context.TODO(), cfg, http.DefaultTransport, true)
	rtest.OK(t, err)

	folderID, err := be.getFolderID(context.TODO(), cfg.Prefix)
	rtest.OK(t, err)
	defer gdriveDeleteItem(context.TODO(), be.service, folderID)

	itemname := "file"
	itempath := path.Join(cfg.Prefix, itemname)

	_, err = gdriveUploadFile(context.TODO(), be.service, folderID, itemname, strings.NewReader("data"), true)
	rtest.OK(t, err)

	rtest.OK(t, be.removeItem(context.TODO(), itempath))
	_, err = gdriveGetItem(context.TODO(), be.service, folderID, itemname)
	rtest.Equals(t, true, err != nil && isNotExist(err))

	// can delete item that does not exist
	rtest.OK(t, be.removeItem(context.TODO(), itempath))
}
