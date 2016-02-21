// +build go1.4

package main

import (
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRouter(t *testing.T) {
	router := NewRouter()

	getConfig := []byte("GET /config")
	router.GetFunc("/config", func(w http.ResponseWriter, r *http.Request) {
		w.Write(getConfig)
	})

	postConfig := []byte("POST /config")
	router.PostFunc("/config", func(w http.ResponseWriter, r *http.Request) {
		w.Write(postConfig)
	})

	getBlobs := []byte("GET /blobs/")
	router.GetFunc("/blobs/", func(w http.ResponseWriter, r *http.Request) {
		w.Write(getBlobs)
	})

	getBlob := []byte("GET /blobs/:sha")
	router.GetFunc("/blobs/:sha", func(w http.ResponseWriter, r *http.Request) {
		w.Write(getBlob)
	})

	server := httptest.NewServer(router)
	defer server.Close()

	getConfigResp, _ := http.Get(server.URL + "/config")
	getConfigBody, _ := ioutil.ReadAll(getConfigResp.Body)
	if getConfigResp.StatusCode != 200 {
		t.Fatalf("Wanted HTTP Status 200, got %d", getConfigResp.StatusCode)
	}
	if string(getConfig) != string(getConfigBody) {
		t.Fatalf("Config wrong:\nWanted '%s'\nGot: '%s'", string(getConfig), string(getConfigBody))
	}

	postConfigResp, _ := http.Post(server.URL+"/config", "binary/octet-stream", strings.NewReader("post test"))
	postConfigBody, _ := ioutil.ReadAll(postConfigResp.Body)
	if postConfigResp.StatusCode != 200 {
		t.Fatalf("Wanted HTTP Status 200, got %d", postConfigResp.StatusCode)
	}
	if string(postConfig) != string(postConfigBody) {
		t.Fatalf("Config wrong:\nWanted '%s'\nGot: '%s'", string(postConfig), string(postConfigBody))
	}

	getBlobsResp, _ := http.Get(server.URL + "/blobs/")
	getBlobsBody, _ := ioutil.ReadAll(getBlobsResp.Body)
	if getBlobsResp.StatusCode != 200 {
		t.Fatalf("Wanted HTTP Status 200, got %d", getBlobsResp.StatusCode)
	}
	if string(getBlobs) != string(getBlobsBody) {
		t.Fatalf("Config wrong:\nWanted '%s'\nGot: '%s'", string(getBlobs), string(getBlobsBody))
	}

	getBlobResp, _ := http.Get(server.URL + "/blobs/test")
	getBlobBody, _ := ioutil.ReadAll(getBlobResp.Body)
	if getBlobResp.StatusCode != 200 {
		t.Fatalf("Wanted HTTP Status 200, got %d", getBlobResp.StatusCode)
	}
	if string(getBlob) != string(getBlobBody) {
		t.Fatalf("Config wrong:\nWanted '%s'\nGot: '%s'", string(getBlob), string(getBlobBody))
	}
}
