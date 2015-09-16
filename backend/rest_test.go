package backend_test

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

	r := http.NewServeMux()

	// Check if a configuration exists.
	r.HandleFunc("/config", func(w http.ResponseWriter, r *http.Request) {
		method := r.Method
		file := filepath.Join(path, "config")
		_, err := os.Stat(file)

		// Check if the config exists
		if method == "HEAD" && err == nil {
			return
		}

		// Get the config
		if method == "GET" && err == nil {
			bytes, _ := ioutil.ReadFile(file)
			w.Write(bytes)
			return
		}

		// Save the config
		if method == "POST" && err != nil {
			bytes, _ := ioutil.ReadAll(r.Body)
			ioutil.WriteFile(file, bytes, 0600)
			return
		}

		http.Error(w, "404 not found", 404)
	})

	for _, dir := range dirs {
		r.HandleFunc("/"+dir+"/", func(w http.ResponseWriter, r *http.Request) {
			method := r.Method
			vars := strings.Split(r.RequestURI, "/")
			name := vars[2]
			path := filepath.Join(path, dir, name)
			_, err := os.Stat(path)

			// List the blobs of a given dir.
			if method == "GET" && name == "" && err == nil {
				files, _ := ioutil.ReadDir(path)
				names := make([]string, len(files))
				for i, f := range files {
					names[i] = f.Name()
				}
				data, _ := json.Marshal(names)
				w.Write(data)
				return
			}

			// Check if the blob esists
			if method == "HEAD" && name != "" && err == nil {
				return
			}

			// Get a blob of a given dir.
			if method == "GET" && name != "" && err == nil {
				file, _ := os.Open(path)
				defer file.Close()
				http.ServeContent(w, r, "", time.Unix(0, 0), file)
				return
			}

			// Save a blob
			if method == "POST" && name != "" && err != nil {
				bytes, _ := ioutil.ReadAll(r.Body)
				ioutil.WriteFile(path, bytes, 0600)
				return
			}

			// Delete a blob
			if method == "DELETE" && name != "" && err == nil {
				os.Remove(path)
				return
			}

			http.Error(w, "404 not found", 404)
		})
	}

	// Start the server and launch the tests.
	s := httptest.NewServer(r)

	defer s.Close()
	u, _ := url.Parse(s.URL)
	backend, _ := rest.Open(u)

	testBackend(backend, t)
}
