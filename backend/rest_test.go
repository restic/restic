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
	// Initializing a temporary repository for the backend
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

	// Routing the repository requests
	r := mux.NewRouter()

	// Exists
	r.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		file := filepath.Join(path, "config")
		if _, err := os.Stat(file); err != nil {
			http.Error(w, "No repository here", 404)
		}
	}).Methods("HEAD")

	// List blobs
	r.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		file := filepath.Join(path, "config")
		if _, err := os.Stat(file); err != nil {
			http.Error(w, "No repository here", 404)
		}
	}).Methods("GET")

	// Head file
	r.HandleFunc("/{file}", func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		file := filepath.Join(path, vars["file"])
		if _, err := os.Stat(file); err != nil {
			http.Error(w, "File not found", 404)
		}
	}).Methods("HEAD")

	// Get file
	r.HandleFunc("/{file}", func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		file := filepath.Join(path, vars["file"])
		if _, err := os.Stat(file); err == nil {
			bytes, _ := ioutil.ReadFile(file)
			w.Write(bytes)
		} else {
			http.Error(w, "File not found", 404)
		}
	}).Methods("GET")

	// Put file
	r.HandleFunc("/{file}", func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		file := filepath.Join(path, vars["file"])
		if _, err := os.Stat(file); err == nil {
			fmt.Fprintf(w, "Blob already uploaded", 404)
		} else {
			bytes, _ := ioutil.ReadAll(r.Body)
			ioutil.WriteFile(file, bytes, 0600)

		}
	}).Methods("POST")

	// Delete file
	r.HandleFunc("/{file}", func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		file := filepath.Join(path, vars["file"])
		if _, err := os.Stat(file); err == nil {
			os.Remove(file)
		} else {
			fmt.Fprintf(w, "File not found", 404)
		}
	}).Methods("DELETE")

	// List blobs
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

	// Head blob
	r.HandleFunc("/{type}/{blob}", func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		blob := filepath.Join(path, vars["type"], vars["blob"])
		if _, err := os.Stat(blob); err != nil {
			http.Error(w, "File not found", 404)
		}
	}).Methods("HEAD")

	// Get blob
	r.HandleFunc("/{type}/{blob}", func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		blob := filepath.Join(path, vars["type"], vars["blob"])
		if _, err := os.Stat(blob); err == nil {
			bytes, _ := ioutil.ReadFile(blob)
			w.Write(bytes)
		} else {
			http.Error(w, "Blob not found", 404)
		}
	}).Methods("GET")

	// Put blob
	r.HandleFunc("/{type}/{blob}", func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		blob := filepath.Join(path, vars["type"], vars["blob"])
		if _, err := os.Stat(blob); err == nil {
			fmt.Fprintf(w, "Blob already uploaded", 404)
		} else {
			bytes, _ := ioutil.ReadAll(r.Body)
			ioutil.WriteFile(blob, bytes, 0600)
		}
	}).Methods("POST")

	// Delete blob
	r.HandleFunc("/{type}/{blob}", func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		blob := filepath.Join(path, vars["type"], vars["blob"])
		if _, err := os.Stat(blob); err == nil {
			os.Remove(blob)
		} else {
			fmt.Fprintf(w, "Blob not found", 404)
		}
	}).Methods("DELETE")

	s := httptest.NewServer(r)
	defer s.Close()

	u, _ := url.Parse(s.URL)
	backend, _ := rest.Open(u)

	testBackend(backend, t)
}
