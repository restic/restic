package main

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Context struct {
	path string
}

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

func CheckConfig(c *Context) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		config := filepath.Join(c.path, "config")
		if _, err := os.Stat(config); err != nil {
			http.Error(w, "404 not found", 404)
			return
		}
	}
}

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

func CheckBlob(c *Context) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := strings.Split(r.RequestURI, "/")
		dir := vars[1]
		name := vars[2]
		path := filepath.Join(c.path, dir, name)
		_, err := os.Stat(path)
		if err != nil {
			http.Error(w, "404 not found", 404)
			return
		}
	}
}

func GetBlob(c *Context) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := strings.Split(r.RequestURI, "/")
		dir := vars[1]
		name := vars[2]
		path := filepath.Join(c.path, dir, name)
		file, err := os.Open(path)
		if err != nil {
			http.Error(w, "404 not found", 404)
			return
		}
		defer file.Close()
		http.ServeContent(w, r, "", time.Unix(0, 0), file)
	}
}

func SaveBlob(c *Context) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := strings.Split(r.RequestURI, "/")
		dir := vars[1]
		name := vars[2]
		path := filepath.Join(c.path, dir, name)
		bytes, err := ioutil.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "400 bad request", 400)
			return
		}
		errw := ioutil.WriteFile(path, bytes, 0600)
		if errw != nil {
			http.Error(w, "500 internal server error", 500)
			return
		}
		w.Write([]byte("200 ok"))
	}
}

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
