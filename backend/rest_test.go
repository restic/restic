package backend_test

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/restic/restic/backend"
	"github.com/restic/restic/backend/rest"
)

func TestRestBackend(t *testing.T) {

	// Initializing a temporary direcory for the rest backend.
	path, _ := ioutil.TempDir("", "restic-repository-")
	defer os.RemoveAll(path)
	dirs := []string{
		path,
		filepath.Join(path, string(backend.Data)),
		filepath.Join(path, string(backend.Snapshot)),
		filepath.Join(path, string(backend.Index)),
		filepath.Join(path, string(backend.Lock)),
		filepath.Join(path, string(backend.Key)),
	}
	for _, d := range dirs {
		os.MkdirAll(d, backend.Modes.Dir)
	}

	r := mux.NewRouter()

	// Check if a configuration exists.
	r.HandleFunc("/config", func(w http.ResponseWriter, r *http.Request) {
		file := filepath.Join(path, "config")
		if _, err := os.Stat(file); err != nil {
			http.Error(w, "404 repository not found", 404)
			return
		}
	}).Methods("HEAD")

	// Get the configuration.
	r.HandleFunc("/config", func(w http.ResponseWriter, r *http.Request) {
		file := filepath.Join(path, "config")
		if _, err := os.Stat(file); err != nil {
			http.Error(w, "404 repository not found", 404)
			return
		}
		bytes, _ := ioutil.ReadFile(file)
		w.Write(bytes)
	}).Methods("GET")

	// Save the configuration.
	r.HandleFunc("/config", func(w http.ResponseWriter, r *http.Request) {
		file := filepath.Join(path, "config")
		if _, err := os.Stat(file); err == nil {
			http.Error(w, "409 repository already initialized", 409)
			return
		}
		bytes, _ := ioutil.ReadAll(r.Body)
		ioutil.WriteFile(file, bytes, 0600)
	}).Methods("POST")

	// List the blobs of a given type.
	r.HandleFunc("/{type}/", func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		blobType, errType := backend.ParseType(filepath.Clean(vars["type"]))
		if errType != nil {
			http.Error(w, "403 invalid blob type", 403)
			return
		}
		path := filepath.Join(path, string(blobType))
		files, _ := ioutil.ReadDir(path)
		names := make([]string, len(files))
		for i, f := range files {
			names[i] = f.Name()
		}
		data, _ := json.Marshal(names)
		w.Write(data)
	}).Methods("GET")

	// Check if a blob of a given type exists.
	r.HandleFunc("/{type}/{blob}", func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		blobType, errType := backend.ParseType(filepath.Clean(vars["type"]))
		if errType != nil {
			http.Error(w, "403 invalid blob type", 403)
			return
		}
		blobID, errID := backend.ParseID(vars["blob"])
		if errID != nil {
			http.Error(w, "403 invalid blob ID", 403)
			return
		}
		blob := filepath.Join(path, string(blobType), blobID.String())
		if _, err := os.Stat(blob); err != nil {
			http.Error(w, "404 blob not found", 404)
		}
	}).Methods("HEAD")

	// Get a blob of a given type.
	r.HandleFunc("/{type}/{blob}", func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		blobType, errType := backend.ParseType(filepath.Clean(vars["type"]))
		if errType != nil {
			http.Error(w, "403 invalid blob type", 403)
			return
		}
		blobID, errID := backend.ParseID(vars["blob"])
		if errID != nil {
			http.Error(w, "403 invalid blob ID", 403)
			return
		}
		blob := filepath.Join(path, string(blobType), blobID.String())
		file, err := os.Open(blob)
		defer file.Close()
		if err != nil {
			http.Error(w, "404 blob not found", 404)
			return
		}
		http.ServeContent(w, r, "", time.Unix(0, 0), file)
	}).Methods("GET")

	// Save a blob of a given type.
	r.HandleFunc("/{type}/{blob}", func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		blobType, errType := backend.ParseType(filepath.Clean(vars["type"]))
		if errType != nil {
			http.Error(w, "403 invalid blob type", 403)
			return
		}
		blobID, errID := backend.ParseID(vars["blob"])
		if errID != nil {
			http.Error(w, "403 invalid blob ID", 403)
			return
		}
		blob := filepath.Join(path, string(blobType), blobID.String())
		if _, err := os.Stat(blob); err == nil {
			http.Error(w, "409 blob already uploaded", 409)
			return
		}
		bytes, _ := ioutil.ReadAll(r.Body)
		ioutil.WriteFile(blob, bytes, 0600)
	}).Methods("POST")

	// Delete a blob of a given type.
	r.HandleFunc("/{type}/{blob}", func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		blobType, errType := backend.ParseType(filepath.Clean(vars["type"]))
		if errType != nil {
			http.Error(w, "403 invalid blob type", 403)
			return
		}
		blobID, errID := backend.ParseID(vars["blob"])
		if errID != nil {
			http.Error(w, "403 invalid blob ID", 403)
			return
		}
		blob := filepath.Join(path, string(blobType), blobID.String())
		if _, err := os.Stat(blob); err != nil {
			http.Error(w, "404 blob not found", 404)
			return
		}
		if err := os.Remove(blob); err != nil {
			fmt.Println(err.Error())
			http.Error(w, "500 internal server error", 500)
			return
		}
	}).Methods("DELETE")

	// Start the server and launch the tests.
	s := httptest.NewServer(r)
	defer s.Close()
	u, _ := url.Parse(s.URL)
	backend, _ := rest.Open(u)

	testBackend(backend, t)
}
