// +build debug

package main

import (
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"
)

var (
	listenMemoryProfile string
)

func init() {
	f := cmdRoot.PersistentFlags()
	f.StringVar(&listenMemoryProfile, "listen-profile", "", "listen on this `address:port` for memory profiling")
}

func runDebug() {
	if listenMemoryProfile != "" {
		fmt.Fprintf(os.Stderr, "running memory profile HTTP server on %v\n", listenMemoryProfile)
		go func() {
			err := http.ListenAndServe(listenMemoryProfile, nil)
			if err != nil {
				fmt.Fprintf(os.Stderr, "memory profile listen failed: %v\n", err)
			}
		}()
	}
}
