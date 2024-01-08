package fs

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

type GoogleDriveFileInfo struct {
	file *drive.File
}

// statically ensure that GoogleDriveFileInfo implements FS.
var _ fs.FileInfo = &GoogleDriveFileInfo{}

func (gd *GoogleDriveFileInfo) IsDir() bool {
	return gd.file.MimeType == "application/vnd.google-apps.folder"
}

func (gd *GoogleDriveFileInfo) ModTime() time.Time {
	modifiedTime, _ := time.Parse(time.RFC3339, gd.file.ModifiedTime)
	return modifiedTime
}

func (gd *GoogleDriveFileInfo) Mode() fs.FileMode {
	switch gd.file.MimeType {
	case "application/vnd.google-apps.folder":
		return fs.ModeDir
	case "application/vnd.google-apps.document", "application/vnd.google-apps.presentation", "application/vnd.google-apps.spreadsheet":
		return os.ModeSocket // ignore for now
	default:
		return 0
	}
}

func (gd *GoogleDriveFileInfo) Name() string {
	return gd.file.Name
}

func (gd *GoogleDriveFileInfo) Size() int64 {
	return gd.file.Size
}

func (*GoogleDriveFileInfo) Sys() any {
	return nil
}

func newGoogleDriveFile(drive *drive.Service, id string) (*GoogleDriveFile, error) {
	file, err := drive.Files.Get(id).Do()
	if err != nil {
		return nil, err
	}

	return &GoogleDriveFile{
		drive,
		file,
		nil,
	}, nil
}

type GoogleDriveFile struct {
	drive *drive.Service
	file  *drive.File
	media *http.Response
}

func (gd *GoogleDriveFile) Close() error {
	if gd.media != nil {
		return gd.media.Body.Close()
	}
	return nil
}

func (*GoogleDriveFile) Fd() uintptr {
	panic("unimplemented")
}

func (f *GoogleDriveFile) Name() string {
	return f.file.Name
}

func (f *GoogleDriveFile) Read(p []byte) (n int, err error) {
	if f.media == nil {
		f.media, err = f.drive.Files.Get(f.file.Id).Download()
		if err != nil {
			return 0, err
		}
	}
	return f.media.Body.Read(p)
}

func (f *GoogleDriveFile) Readdir(int) ([]fs.FileInfo, error) {
	search, err := f.drive.Files.List().Q("'" + f.file.Id + "' in parents").Fields("files(id, name, mimeType, modifiedTime)").Do()
	if err != nil {
		return nil, err
	}
	result := make([]fs.FileInfo, len(search.Files))
	for i, child := range search.Files {
		result[i] = &GoogleDriveFileInfo{child}
	}
	return result, nil
}

func (f *GoogleDriveFile) Readdirnames(n int) ([]string, error) {
	search, err := f.drive.Files.List().Q("'" + f.file.Id + "' in parents").Fields("files(id)").Do()
	if err != nil {
		return nil, err
	}
	result := make([]string, len(search.Files))
	for i, child := range search.Files {
		result[i] = child.Id
	}
	return result, nil
}

func (*GoogleDriveFile) Seek(int64, int) (int64, error) {
	panic("unimplemented")
}

func (f *GoogleDriveFile) Stat() (fs.FileInfo, error) {
	return &GoogleDriveFileInfo{f.file}, nil
}

var _ File = &GoogleDriveFile{}

type GoogleDrive struct {
	drive *drive.Service
}

// statically ensure that GoogleDrive implements FS.
var _ FS = &GoogleDrive{}

func NewGoogleDrive() *GoogleDrive {
	ctx := context.Background()
	b, err := os.ReadFile("credentials.json")
	if err != nil {
		log.Fatalf("Unable to read client secret file: %v", err)
	}
	// If modifying these scopes, delete your previously saved token.json.
	config, err := google.ConfigFromJSON(b, drive.DriveScope)
	if err != nil {
		log.Fatalf("Unable to parse client secret file to config: %v", err)
	}
	client := getClient(config)

	srv, err := drive.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		log.Fatalf("Unable to retrieve Drive client: %v", err)
	}

	return &GoogleDrive{
		drive: srv,
	}
}

// Retrieve a token, saves the token, then returns the generated client.
func getClient(config *oauth2.Config) *http.Client {
	// The file token.json stores the user's access and refresh tokens, and is
	// created automatically when the authorization flow completes for the first
	// time.
	tokFile := "token.json"
	tok, err := tokenFromFile(tokFile)
	if err != nil {
		tok = getTokenFromWeb(config)
		saveToken(tokFile, tok)
	}
	return config.Client(context.Background(), tok)
}

// Request a token from the web, then returns the retrieved token.
func getTokenFromWeb(config *oauth2.Config) *oauth2.Token {
	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	fmt.Printf("Go to the following link in your browser then type the "+
		"authorization code: \n%v\n", authURL)

	var authCode string
	if _, err := fmt.Scan(&authCode); err != nil {
		log.Fatalf("Unable to read authorization code %v", err)
	}

	tok, err := config.Exchange(context.TODO(), authCode)
	if err != nil {
		log.Fatalf("Unable to retrieve token from web %v", err)
	}
	return tok
}

// Retrieves a token from a local file.
func tokenFromFile(file string) (*oauth2.Token, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	tok := &oauth2.Token{}
	err = json.NewDecoder(f).Decode(tok)
	return tok, err
}

// Saves a token to a file path.
func saveToken(path string, token *oauth2.Token) {
	fmt.Printf("Saving credential file to: %s\n", path)
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		log.Fatalf("Unable to cache oauth token: %v", err)
	}
	defer f.Close()
	json.NewEncoder(f).Encode(token)
}

func (*GoogleDrive) Abs(path string) (string, error) {
	return path, nil
}

func (*GoogleDrive) Base(path string) string {
	return filepath.Base(path)
}

func (*GoogleDrive) Clean(path string) string {
	return path
}

func (*GoogleDrive) Dir(path string) string {
	return filepath.Dir(path)
}

func (*GoogleDrive) IsAbs(path string) bool {
	return true
}

func (*GoogleDrive) Join(elem ...string) string {
	return elem[len(elem)-1]
}

func (gd *GoogleDrive) Lstat(id string) (fs.FileInfo, error) {
	file, err := gd.drive.Files.Get(id).Do()
	if err != nil {
		return nil, err
	}

	return &GoogleDriveFileInfo{
		file: file,
	}, nil
}

func (gd *GoogleDrive) Open(id string) (File, error) {
	return newGoogleDriveFile(gd.drive, id)
}

func (gd *GoogleDrive) OpenFile(name string, flag int, perm fs.FileMode) (File, error) {
	return gd.Open(name)
}

func (*GoogleDrive) Separator() string {
	return "/"
}

func (*GoogleDrive) Stat(id string) (fs.FileInfo, error) {
	return Lstat(id)
}

func (*GoogleDrive) VolumeName(path string) string {
	return ""
}
