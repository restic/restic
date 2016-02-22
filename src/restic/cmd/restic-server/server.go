// +build go1.4

package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"path/filepath"
)

const (
	defaultHTTPPort  = ":8000"
	defaultHTTPSPort = ":8443"
)

func main() {
	// Parse command-line args
	var path = flag.String("path", "/tmp/restic", "specifies the path of the data directory")
	var tls = flag.Bool("tls", false, "turns on tls support")
	flag.Parse()

	// Create the missing directories
	dirs := []string{
		"data",
		"snapshots",
		"index",
		"locks",
		"keys",
		"tmp",
	}
	for _, d := range dirs {
		os.MkdirAll(filepath.Join(*path, d), 0700)
	}

	// Define the routes
	context := &Context{*path}
	router := NewRouter()
	router.HeadFunc("/config", CheckConfig(context))
	router.GetFunc("/config", GetConfig(context))
	router.PostFunc("/config", SaveConfig(context))
	router.GetFunc("/:dir/", ListBlobs(context))
	router.HeadFunc("/:dir/:name", CheckBlob(context))
	router.GetFunc("/:type/:name", GetBlob(context))
	router.PostFunc("/:type/:name", SaveBlob(context))
	router.DeleteFunc("/:type/:name", DeleteBlob(context))

	// Check for a password file
	var handler http.Handler
	htpasswdFile, err := NewHtpasswdFromFile(filepath.Join(*path, ".htpasswd"))
	if err != nil {
		log.Println("Authentication disabled")
		handler = router
	} else {
		log.Println("Authentication enabled")
		handler = AuthHandler(htpasswdFile, router)
	}

	// start the server
	if !*tls {
		log.Printf("start server on port %s\n", defaultHTTPPort)
		http.ListenAndServe(defaultHTTPPort, handler)
	} else {
		privateKey := filepath.Join(*path, "private_key")
		publicKey := filepath.Join(*path, "public_key")
		log.Println("TLS enabled")
		log.Printf("private key: %s", privateKey)
		log.Printf("public key: %s", publicKey)
		log.Printf("start server on port %s\n", defaultHTTPSPort)
		http.ListenAndServeTLS(defaultHTTPSPort, publicKey, privateKey, handler)
	}
}
