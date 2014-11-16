// +build debug_cmd

package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"
)

var debugLogger = initDebugLogger()

func initDebugLogger() *log.Logger {
	// create new log file
	filename := fmt.Sprintf("khepri-debug-%d-%s",
		os.Getpid(), time.Now().Format("20060201-150405"))
	path := filepath.Join(os.TempDir(), filename)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE, 0600)
	if err != nil {
		fmt.Fprintf(os.Stderr, "unable to create debug log file: %v", err)
		os.Exit(2)
	}

	// open logger
	l := log.New(io.MultiWriter(os.Stderr, f), "DEBUG: ", log.LstdFlags)
	fmt.Fprintf(os.Stderr, "debug log for khepri command activated, writing log file %s\n", path)
	l.Printf("khepri %s", version)

	return l
}

func debug(fmt string, args ...interface{}) {
	debugLogger.Printf(fmt, args...)
}
