// +build go1.4

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"restic/fs"
)

// Context contains repository meta-data.
type Context struct {
	path string
}

// AuthHandler wraps h with a http.HandlerFunc that performs basic
// authentication against the user/passwords pairs stored in f and returns the
// http.HandlerFunc.
func AuthHandler(f *HtpasswdFile, h http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		username, password, ok := r.BasicAuth()
		if !ok {
			http.Error(w, "401 unauthorized", 401)
			return
		}
		if !f.Validate(username, password) {
			http.Error(w, "401 unauthorized", 401)
			return
		}
		h.ServeHTTP(w, r)
	}
}

// CheckConfig returns a http.HandlerFunc that checks whether
// a configuration exists.
func CheckConfig(c *Context) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		config := filepath.Join(c.path, "config")
		st, err := os.Stat(config)
		if err != nil {
			http.Error(w, "404 not found", 404)
			return
		}
		w.Header().Add("Content-Length", fmt.Sprint(st.Size()))
	}
}

// GetConfig returns a http.HandlerFunc that allows for a
// config to be retrieved.
func GetConfig(c *Context) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		config := filepath.Join(c.path, "config")
		bytes, err := ioutil.ReadFile(config)
		if err != nil {
			http.Error(w, "404 not found", 404)
			return
		}
		w.Write(bytes)
	}
}

// SaveConfig returns a http.HandlerFunc that allows for a
// config to be saved.
func SaveConfig(c *Context) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		config := filepath.Join(c.path, "config")
		bytes, err := ioutil.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "400 bad request", 400)
			return
		}
		errw := ioutil.WriteFile(config, bytes, 0600)
		if errw != nil {
			http.Error(w, "500 internal server error", 500)
			return
		}
		w.Write([]byte("200 ok"))
	}
}

// ListBlobs returns a http.HandlerFunc that lists
// all blobs of a given type in an arbitrary order.
func ListBlobs(c *Context) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := strings.Split(r.RequestURI, "/")
		dir := vars[1]
		path := filepath.Join(c.path, dir)
		files, err := ioutil.ReadDir(path)
		if err != nil {
			http.Error(w, "404 not found", 404)
			return
		}
		names := make([]string, len(files))
		for i, f := range files {
			names[i] = f.Name()
		}
		data, err := json.Marshal(names)
		if err != nil {
			http.Error(w, "500 internal server error", 500)
			return
		}
		w.Write(data)
	}
}

// CheckBlob reutrns a http.HandlerFunc that tests whether a blob exists
// and returns 200, if it does, or 404 otherwise.
func CheckBlob(c *Context) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := strings.Split(r.RequestURI, "/")
		dir := vars[1]
		name := vars[2]
		path := filepath.Join(c.path, dir, name)
		st, err := os.Stat(path)
		if err != nil {
			http.Error(w, "404 not found", 404)
			return
		}
		w.Header().Add("Content-Length", fmt.Sprint(st.Size()))
	}
}

// GetBlob returns a http.HandlerFunc that retrieves a blob
// from the repository.
func GetBlob(c *Context) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := strings.Split(r.RequestURI, "/")
		dir := vars[1]
		name := vars[2]
		path := filepath.Join(c.path, dir, name)
		file, err := fs.Open(path)
		if err != nil {
			http.Error(w, "404 not found", 404)
			return
		}
		defer file.Close()
		http.ServeContent(w, r, "", time.Unix(0, 0), file)
	}
}

// SaveBlob returns a http.HandlerFunc that saves a blob to the repository.
func SaveBlob(c *Context) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := strings.Split(r.RequestURI, "/")
		dir := vars[1]
		name := vars[2]
		path := filepath.Join(c.path, dir, name)
		tmp := path + "_tmp"
		tf, err := fs.OpenFile(tmp, os.O_CREATE|os.O_WRONLY, 0600)
		if err != nil {
			http.Error(w, "500 internal server error", 500)
			return
		}
		if _, err := io.Copy(tf, r.Body); err != nil {
			http.Error(w, "400 bad request", 400)
			tf.Close()
			os.Remove(tmp)
			return
		}
		if err := tf.Close(); err != nil {
			http.Error(w, "500 internal server error", 500)
		}
		if err := os.Rename(tmp, path); err != nil {
			http.Error(w, "500 internal server error", 500)
			return
		}
		w.Write([]byte("200 ok"))
	}
}

// DeleteBlob returns a http.HandlerFunc that deletes a blob from the
// repository.
func DeleteBlob(c *Context) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := strings.Split(r.RequestURI, "/")
		dir := vars[1]
		name := vars[2]
		path := filepath.Join(c.path, dir, name)
		err := os.Remove(path)
		if err != nil {
			http.Error(w, "500 internal server error", 500)
			return
		}
		w.Write([]byte("200 ok"))
	}
}
