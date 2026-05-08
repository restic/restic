package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"runtime"

	"github.com/spf13/cobra"
)

const (
	ResticDocsURL    string = "https://restic.readthedocs.io/en/stable"
	ResticDevDocsURL string = "https://restic.readthedocs.io/en/latest"
)

type execFn func(name string, arg ...string) *exec.Cmd

var (
	stdout io.Writer = os.Stdout
	start  execFn    = exec.Command
)

func newDocsCommand() *cobra.Command {

	var cmd = &cobra.Command{
		Use:   "docs",
		Short: "Opens the documentation in the default browser",
		Run: func(_ *cobra.Command, _ []string) {
			openDocs(ResticDocsURL, "user")
		},
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "user",
		Short: "Show the user documentation",
		Run: func(_ *cobra.Command, _ []string) {
			openDocs(ResticDocsURL, "user")
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "dev",
		Short: "Show the developer documentation",
		Run: func(_ *cobra.Command, _ []string) {
			openDocs(ResticDevDocsURL, "developer")
		},
	})

	return cmd
}

func openDocs(url string, docType string) {
	_, _ = fmt.Fprintf(stdout, "Opening the %s documentation at %s\n", docType, url)

	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "linux":
		cmd = start("xdg-open", url)
	case "windows":
		cmd = start("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = start("open", url)
	default:
		log.Fatalf("Unsupported platform: %s", runtime.GOOS)
	}

	if err := cmd.Start(); err != nil {
		log.Fatalf("Failed to open brower: %v", err)
	}
}
