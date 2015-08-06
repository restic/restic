package backend_test

import (
	"encoding/json"
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

func setupRestBackend(t *testing.T) *rest.Rest {
	url, _ := url.Parse("http://localhost:8000")
	backend, _ := rest.Open(url)
	return backend
}

func TestRestBackend(t *testing.T) {

	// Initializing a temporary direcory for the rest backend.
	path, _ := ioutil.TempDir("", "restic-repository-")
	defer os.RemoveAll(path)

	dirs := []string{
		path,
		filepath.Join(path, "data"),
		filepath.Join(path, "snapshot"),
		filepath.Join(path, "index"),
		filepath.Join(path, "lock"),
		filepath.Join(path, "key"),
		filepath.Join(path, "temp"),
	}

	for _, d := range dirs {
		os.MkdirAll(d, backend.Modes.Dir)
	}

	// Initialize the router for the repository requests.
	r := mux.NewRouter()

	// Check if the repository has already been initialized.
	r.HandleFunc("/config", func(w http.ResponseWriter, r *http.Request) {
		file := filepath.Join(path, "config")
		if _, err := os.Stat(file); err != nil {
			http.Error(w, "Repository not found", 404)
		}
	}).Methods("HEAD")

	// Get the configuration of the repository.
	r.HandleFunc("/config", func(w http.ResponseWriter, r *http.Request) {
		file := filepath.Join(path, "config")
		if _, err := os.Stat(file); err == nil {
			bytes, _ := ioutil.ReadFile(file)
			w.Write(bytes)
		} else {
			http.Error(w, "Repository not found", 404)
		}
	}).Methods("GET")

	// Initialize the repository and save the configuration.
	r.HandleFunc("/config", func(w http.ResponseWriter, r *http.Request) {
		file := filepath.Join(path, "config")
		if _, err := os.Stat(file); err == nil {
			http.Error(w, "Repository already initialized", 403)
		} else {
			bytes, _ := ioutil.ReadAll(r.Body)
			ioutil.WriteFile(file, bytes, 0600)

		}
	}).Methods("POST")

	// List the blobs of a given type.
	r.HandleFunc("/{type}/", func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		path := filepath.Join(path, vars["type"])
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
		blob := filepath.Join(path, vars["type"], vars["blob"])
		if _, err := os.Stat(blob); err != nil {
			http.Error(w, "Blob not found", 404)
		}
	}).Methods("HEAD")

	// Get a blob of a given type.
	r.HandleFunc("/{type}/{blob}", func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		blob := filepath.Join(path, vars["type"], vars["blob"])
		if file, err := os.Open(blob); err == nil {
			http.ServeContent(w, r, "", time.Unix(0, 0), file)
		} else {
			http.Error(w, "Blob not found", 404)
		}
	}).Methods("GET")

	// Save a blob of a given type.
	r.HandleFunc("/{type}/{blob}", func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		blob := filepath.Join(path, vars["type"], vars["blob"])
		if _, err := os.Stat(blob); err == nil {
			http.Error(w, "Blob already uploaded", 403)
		} else {
			bytes, _ := ioutil.ReadAll(r.Body)
			ioutil.WriteFile(blob, bytes, 0600)
		}
	}).Methods("POST")

	// Delete a blob of a given type.
	r.HandleFunc("/{type}/{blob}", func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		blob := filepath.Join(path, vars["type"], vars["blob"])
		if _, err := os.Stat(blob); err == nil {
			os.Remove(blob)
		} else {
			http.Error(w, "Blob not found", 404)
		}
	}).Methods("DELETE")

	// Start the server and launch the tests.
	s := httptest.NewServer(r)
	defer s.Close()

	u, _ := url.Parse(s.URL)
	backend, _ := rest.Open(u)

	testBackend(backend, t)
}
