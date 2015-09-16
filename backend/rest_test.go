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

func TestRestBackend(t *testing.T) {

	// Initialize a temporary direcory for the rest backend.
	path, _ := ioutil.TempDir("", "restic-repository-")
	defer os.RemoveAll(path)

	// Create all the necessary subdirectories
	dirs := []string{
		backend.Paths.Data,
		backend.Paths.Snapshots,
		backend.Paths.Index,
		backend.Paths.Locks,
		backend.Paths.Keys,
	}
	for _, d := range dirs {
		os.MkdirAll(filepath.Join(path, d), backend.Modes.Dir)
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

	// List the blobs of a given dir.
	r.HandleFunc("/{dir}/", func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		dir := filepath.Clean(vars["dir"])
		path := filepath.Join(path, dir)
		files, _ := ioutil.ReadDir(path)
		names := make([]string, len(files))
		for i, f := range files {
			names[i] = f.Name()
		}
		data, _ := json.Marshal(names)
		w.Write(data)
	}).Methods("GET")

	// Check if a blob of a given dir exists.
	r.HandleFunc("/{dir}/{name}", func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		dir := filepath.Clean(vars["dir"])
		name := filepath.Clean(vars["name"])
		path := filepath.Join(path, dir, name)
		if _, err := os.Stat(path); err != nil {
			http.Error(w, "404 blob not found", 404)
		}
	}).Methods("HEAD")

	// Get a blob of a given dir.
	r.HandleFunc("/{dir}/{name}", func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		dir := filepath.Clean(vars["dir"])
		name := filepath.Clean(vars["name"])
		path := filepath.Join(path, dir, name)
		file, err := os.Open(path)
		defer file.Close()
		if err != nil {
			http.Error(w, "404 blob not found", 404)
			return
		}
		http.ServeContent(w, r, "", time.Unix(0, 0), file)
	}).Methods("GET")

	// Save a blob of a given dir.
	r.HandleFunc("/{dir}/{name}", func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		dir := filepath.Clean(vars["dir"])
		name := filepath.Clean(vars["name"])
		path := filepath.Join(path, dir, name)
		if _, err := os.Stat(path); err == nil {
			http.Error(w, "409 blob already uploaded", 409)
			return
		}
		bytes, _ := ioutil.ReadAll(r.Body)
		ioutil.WriteFile(path, bytes, 0600)
	}).Methods("POST")

	// Delete a blob of a given dir.
	r.HandleFunc("/{dir}/{name}", func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		dir := filepath.Clean(vars["dir"])
		name := filepath.Clean(vars["name"])
		path := filepath.Join(path, dir, name)
		if _, err := os.Stat(path); err != nil {
			http.Error(w, "404 blob not found", 404)
			return
		}
		if err := os.Remove(path); err != nil {
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
