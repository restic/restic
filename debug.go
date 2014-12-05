// +build debug

package restic

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
	filename := fmt.Sprintf("restic-lib-debug-%d-%s",
		os.Getpid(), time.Now().Format("20060201-150405"))
	path := filepath.Join(os.TempDir(), filename)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE, 0600)
	if err != nil {
		fmt.Fprintf(os.Stderr, "unable to create debug log file: %v", err)
		os.Exit(2)
	}

	// open logger
	l := log.New(io.MultiWriter(os.Stderr, f), "DEBUG: ", log.LstdFlags)
	fmt.Fprintf(os.Stderr, "debug log for restic library activated, writing log file %s\n", path)

	return l
}

func debug(fmt string, args ...interface{}) {
	debugLogger.Printf(fmt, args...)
}
