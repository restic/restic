package drime_test

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/backend/drime"
	"github.com/restic/restic/internal/backend/test"
	"github.com/restic/restic/internal/options"
)

const fakeToken = "test-token"

type fakeEntry struct {
	id     int
	name   string
	parent int
	folder bool
	data   []byte
}

type fakeUpload struct {
	parent int
	name   string
	parts  map[int][]byte
}

// fakeDrime implements the parts of the Drime API used by the backend.
type fakeDrime struct {
	mu      sync.Mutex
	nextID  int
	entries map[int]*fakeEntry
	uploads map[string]*fakeUpload
	corrupt bool // report a wrong hash when verifying
	srv     *httptest.Server
}

func newFakeDrime() *fakeDrime {
	f := &fakeDrime{nextID: 1, entries: map[int]*fakeEntry{}, uploads: map[string]*fakeUpload{}}
	f.srv = httptest.NewServer(f)
	return f
}

func (f *fakeDrime) find(parent int, name string) *fakeEntry {
	for _, e := range f.entries {
		if e.parent == parent && e.name == name {
			return e
		}
	}
	return nil
}

func (f *fakeDrime) add(parent int, name string, folder bool, data []byte) *fakeEntry {
	// like Drime, uploading to an existing name replaces the file
	if old := f.find(parent, name); old != nil && !folder {
		delete(f.entries, old.id)
	}
	e := &fakeEntry{id: f.nextID, name: name, parent: parent, folder: folder, data: data}
	f.nextID++
	f.entries[e.id] = e
	return e
}

func (f *fakeDrime) remove(id int) {
	for _, e := range f.entries {
		if e.parent == id {
			f.remove(e.id)
		}
	}
	delete(f.entries, id)
}

func jsonOut(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func entryJSON(e *fakeEntry) map[string]any {
	typ := "text"
	if e.folder {
		typ = "folder"
	}
	return map[string]any{"id": e.id, "name": e.name, "type": typ, "file_size": len(e.data), "parent_id": e.parent}
}

func (f *fakeDrime) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p := r.URL.Path

	// endpoints on the "storage" side do not use Drime credentials
	switch {
	case strings.HasPrefix(p, "/_blob/"):
		f.serveBlob(w, r)
		return
	case strings.HasPrefix(p, "/_part/"):
		f.servePart(w, r)
		return
	}

	if r.Header.Get("Authorization") != "Bearer "+fakeToken {
		jsonOut(w, http.StatusUnauthorized, map[string]string{"message": "Unauthenticated."})
		return
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	switch {
	case p == "/api/v1/drive/file-entries" && r.Method == http.MethodGet:
		f.list(w, r)
	case p == "/api/v1/folders" && r.Method == http.MethodPost:
		f.mkdir(w, r)
	case p == "/api/v1/uploads" && r.Method == http.MethodPost:
		f.upload(w, r)
	case p == "/api/v1/file-entries/delete" && r.Method == http.MethodPost:
		f.delete(w, r)
	case p == "/api/v1/s3/multipart/create":
		f.mpCreate(w, r)
	case p == "/api/v1/s3/multipart/batch-sign-part-urls":
		f.mpSign(w, r)
	case p == "/api/v1/s3/multipart/complete":
		jsonOut(w, 200, map[string]string{"status": "success"})
	case p == "/api/v1/s3/multipart/abort":
		var in struct{ Key string }
		_ = json.NewDecoder(r.Body).Decode(&in)
		delete(f.uploads, in.Key)
		jsonOut(w, 200, map[string]string{"status": "success"})
	case p == "/api/v1/s3/entries":
		f.mpEntry(w, r)
	case strings.HasSuffix(p, "/verify-integrity") && r.Method == http.MethodPost:
		f.verify(w, r)
	case strings.HasPrefix(p, "/api/v1/file-entries/") && r.Method == http.MethodGet:
		id, _ := strconv.Atoi(path.Base(p))
		if f.entries[id] == nil || f.entries[id].folder {
			jsonOut(w, 404, map[string]string{"message": "not found"})
			return
		}
		http.Redirect(w, r, "/_blob/"+strconv.Itoa(id), http.StatusFound)
	default:
		jsonOut(w, 404, map[string]string{"message": "unknown endpoint " + p})
	}
}

func (f *fakeDrime) verify(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(path.Base(path.Dir(r.URL.Path)))
	var in struct{ SHA256 string }
	_ = json.NewDecoder(r.Body).Decode(&in)
	e := f.entries[id]
	if e == nil {
		jsonOut(w, 404, map[string]string{"message": "not found"})
		return
	}
	data := e.data
	if f.corrupt {
		data = append([]byte("x"), data...)
	}
	sum := fmt.Sprintf("%x", sha256.Sum256(data))
	jsonOut(w, 200, map[string]any{"status": "success", "verified": sum == in.SHA256, "serverHash": sum})
}

func (f *fakeDrime) list(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	parent, _ := strconv.Atoi(q.Get("folderId"))
	perPage, _ := strconv.Atoi(q.Get("perPage"))
	page, _ := strconv.Atoi(q.Get("page"))
	if perPage <= 0 || perPage > 100 {
		perPage = 100
	}
	if page < 1 {
		page = 1
	}

	var all []*fakeEntry
	for _, e := range f.entries {
		if e.parent == parent && strings.Contains(e.name, q.Get("query")) {
			all = append(all, e)
		}
	}
	sort.Slice(all, func(i, j int) bool { return all[i].id < all[j].id })

	last := max((len(all)+perPage-1)/perPage, 1)
	// the real API serves the last page again for pages out of range
	cur := min(page, last)
	data := []map[string]any{}
	for i := (cur - 1) * perPage; i < min(cur*perPage, len(all)); i++ {
		data = append(data, entryJSON(all[i]))
	}
	jsonOut(w, 200, map[string]any{"current_page": cur, "last_page": last, "data": data})
}

func (f *fakeDrime) mkdir(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name     string
		ParentID *int
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	parent := 0
	if in.ParentID != nil {
		parent = *in.ParentID
	}
	if f.find(parent, in.Name) != nil {
		jsonOut(w, 422, map[string]string{"message": "Folder with same name already exists"})
		return
	}
	jsonOut(w, 200, map[string]any{"folder": entryJSON(f.add(parent, in.Name, true, nil))})
}

func (f *fakeDrime) upload(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		jsonOut(w, 400, map[string]string{"message": err.Error()})
		return
	}
	parent, _ := strconv.Atoi(r.FormValue("parentId"))
	file, hdr, err := r.FormFile("file")
	if err != nil {
		jsonOut(w, 422, map[string]string{"message": "file missing"})
		return
	}
	data, _ := io.ReadAll(file)
	if parent != 0 && f.entries[parent] == nil {
		jsonOut(w, 422, map[string]string{"message": "parent missing"})
		return
	}
	jsonOut(w, 201, map[string]any{"fileEntry": entryJSON(f.add(parent, hdr.Filename, false, data))})
}

func (f *fakeDrime) delete(w http.ResponseWriter, r *http.Request) {
	var in struct {
		EntryIDs      []int
		DeleteForever bool
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	for _, id := range in.EntryIDs {
		if f.entries[id] == nil {
			jsonOut(w, 422, map[string]any{"message": "selected entry ids is invalid"})
			return
		}
	}
	for _, id := range in.EntryIDs {
		f.remove(id)
	}
	jsonOut(w, 200, map[string]string{"status": "success"})
}

func (f *fakeDrime) mpCreate(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Filename  string
		Extension string
		ParentID  *int
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	if in.Extension == "" {
		jsonOut(w, 422, map[string]string{"message": "extension is required"})
		return
	}
	parent := 0
	if in.ParentID != nil {
		parent = *in.ParentID
	}
	key := fmt.Sprintf("uploads/%d-%s", len(f.uploads)+1, in.Filename)
	f.uploads[key] = &fakeUpload{parent: parent, name: in.Filename, parts: map[int][]byte{}}
	jsonOut(w, 200, map[string]string{"uploadId": "up-" + key, "key": key})
}

func (f *fakeDrime) mpSign(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Key         string
		PartNumbers []int
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	urls := []map[string]any{}
	for _, n := range in.PartNumbers {
		urls = append(urls, map[string]any{"partNumber": n, "url": fmt.Sprintf("%s/_part/%d?key=%s", f.srv.URL, n, in.Key)})
	}
	jsonOut(w, 200, map[string]any{"urls": urls})
}

func (f *fakeDrime) servePart(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	n, _ := strconv.Atoi(path.Base(r.URL.Path))
	up := f.uploads[r.URL.Query().Get("key")]
	if up == nil || r.Method != http.MethodPut {
		http.Error(w, "bad part", 400)
		return
	}
	data, _ := io.ReadAll(r.Body)
	up.parts[n] = data
	w.Header().Set("ETag", fmt.Sprintf("%q", fmt.Sprintf("etag-%d", n)))
}

func (f *fakeDrime) mpEntry(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Filename string
		ParentID *int
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	for key, up := range f.uploads {
		if path.Base(key) != in.Filename {
			continue
		}
		var data []byte
		for i := 1; i <= len(up.parts); i++ {
			data = append(data, up.parts[i]...)
		}
		delete(f.uploads, key)
		jsonOut(w, 200, map[string]any{"fileEntry": entryJSON(f.add(up.parent, up.name, false, data))})
		return
	}
	jsonOut(w, 422, map[string]string{"message": "unknown upload"})
}

func (f *fakeDrime) serveBlob(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	id, _ := strconv.Atoi(path.Base(r.URL.Path))
	e := f.entries[id]
	var data []byte
	if e != nil {
		data = bytes.Clone(e.data)
	}
	f.mu.Unlock()
	if e == nil {
		http.Error(w, "gone", 404)
		return
	}
	http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(data))
}

func newFakeSuite(f *fakeDrime) *test.Suite[drime.Config] {
	return &test.Suite[drime.Config]{
		MinimalData: true,
		NewConfig: func() (*drime.Config, error) {
			cfg, err := drime.ParseConfig("drime:restic/test")
			if err != nil {
				return nil, err
			}
			cfg.AccessToken = options.NewSecretString(fakeToken)
			cfg.Endpoint = f.srv.URL + "/api/v1"
			cfg.Path = fmt.Sprintf("restic/test-%d", time.Now().UnixNano())
			return cfg, nil
		},
		Factory: drime.NewFactory(),
	}
}

func TestBackendFakeServer(t *testing.T) {
	f := newFakeDrime()
	defer f.srv.Close()
	newFakeSuite(f).RunTests(t)
}

func TestLargeUpload(t *testing.T) {
	f := newFakeDrime()
	defer f.srv.Close()
	cfg, _ := newFakeSuite(f).NewConfig()
	be, err := drime.NewFactory().Create(t.Context(), cfg, http.DefaultTransport, nil, func(string, ...any) {})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = be.Close() }()

	// larger than a single upload and not a multiple of the part size
	data := make([]byte, 5*1024*1024+8*1024*1024+123)
	_, _ = rand.Read(data)
	saveAndCheck(t, be, data)
}

func saveAndCheck(t *testing.T, be backend.Backend, data []byte) {
	t.Helper()
	ctx := t.Context()
	h := backend.Handle{Type: backend.PackFile, Name: "abcdef0123456789"}

	if err := be.Save(ctx, h, backend.NewByteReader(data, be.Hasher())); err != nil {
		t.Fatal(err)
	}
	fi, err := be.Stat(ctx, h)
	if err != nil || fi.Size != int64(len(data)) {
		t.Fatalf("stat: %v, size %d, want %d", err, fi.Size, len(data))
	}

	load := func(length int, offset int64) []byte {
		var got []byte
		err := be.Load(ctx, h, length, offset, func(rd io.Reader) error {
			var err error
			got, err = io.ReadAll(rd)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	if !bytes.Equal(load(0, 0), data) {
		t.Error("full load differs")
	}
	off := int64(len(data) - 1000)
	if !bytes.Equal(load(500, off), data[off:off+500]) {
		t.Error("range load differs")
	}
	if !bytes.Equal(load(0, off), data[off:]) {
		t.Error("open ended range load differs")
	}

	if err := be.Remove(ctx, h); err != nil {
		t.Fatal(err)
	}
	if _, err := be.Stat(ctx, h); !be.IsNotExist(err) {
		t.Fatalf("expected not exist, got %v", err)
	}
}

func TestCorruptedUploadIsRejected(t *testing.T) {
	f := newFakeDrime()
	defer f.srv.Close()
	cfg, _ := newFakeSuite(f).NewConfig()
	be, err := drime.NewFactory().Create(t.Context(), cfg, http.DefaultTransport, nil, func(string, ...any) {})
	if err != nil {
		t.Fatal(err)
	}

	f.mu.Lock()
	f.corrupt = true
	f.mu.Unlock()

	h := backend.Handle{Type: backend.PackFile, Name: "abcdef0123456789"}
	err = be.Save(t.Context(), h, backend.NewByteReader([]byte("data"), be.Hasher()))
	if err == nil || !be.IsPermanentError(err) {
		t.Fatalf("expected permanent corruption error, got %v", err)
	}
	if _, err := be.Stat(t.Context(), h); !be.IsNotExist(err) {
		t.Fatalf("corrupted file was not removed: %v", err)
	}
}
