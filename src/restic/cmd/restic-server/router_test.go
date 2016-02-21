package main

import (
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
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
	require.Equal(t, 200, getConfigResp.StatusCode)
	require.Equal(t, string(getConfig), string(getConfigBody))

	postConfigResp, _ := http.Post(server.URL+"/config", "binary/octet-stream", strings.NewReader("post test"))
	postConfigBody, _ := ioutil.ReadAll(postConfigResp.Body)
	require.Equal(t, 200, postConfigResp.StatusCode)
	require.Equal(t, string(postConfig), string(postConfigBody))

	getBlobsResp, _ := http.Get(server.URL + "/blobs/")
	getBlobsBody, _ := ioutil.ReadAll(getBlobsResp.Body)
	require.Equal(t, 200, getBlobsResp.StatusCode)
	require.Equal(t, string(getBlobs), string(getBlobsBody))

	getBlobResp, _ := http.Get(server.URL + "/blobs/test")
	getBlobBody, _ := ioutil.ReadAll(getBlobResp.Body)
	require.Equal(t, 200, getBlobResp.StatusCode)
	require.Equal(t, string(getBlob), string(getBlobBody))
}
