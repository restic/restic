package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"regexp"
	"runtime"

	"github.com/spf13/cobra"

	"github.com/restic/restic/internal/global"
)

const (
	GOOS      string = runtime.GOOS
	ResticURL string = "https://restic.readthedocs.io/en"
)

type execFn func(name string, arg ...string) *exec.Cmd

var (
	stdout io.Writer = os.Stdout
	start  execFn    = exec.Command
)

func newDocsCommand(globalOptions *global.Options) *cobra.Command {
	_ = globalOptions

	cmd := &cobra.Command{
		Use:   "docs",
		Short: "Opens the documentation in the default browser",
		Run: func(_ *cobra.Command, _ []string) {
			docsURL := docsURLForVersion(global.Version)
			openDocs(GOOS, docsURL, "user")
		},
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "user",
		Short: "Show the user documentation",
		Run: func(_ *cobra.Command, _ []string) {
			docsURL := fmt.Sprintf("%s/stable", ResticURL)
			openDocs(GOOS, docsURL, "user")
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "dev",
		Short: "Show the developer documentation",
		Run: func(_ *cobra.Command, _ []string) {
			docsURL := fmt.Sprintf("%s/latest", ResticURL)
			openDocs(GOOS, docsURL, "developer")
		},
	})

	return cmd
}

func docsURLForVersion(version string) string {
	extractVersion := func(version string) string {
		// match a semantic version at the start of the string, e.g. "0.18.1" or
		// "0.18.1-dev (compiled manually)" -> capture "0.18.1"
		versionRegex := regexp.MustCompile(`^(\d+\.\d+\.\d+)`)
		matches := versionRegex.FindStringSubmatch(version)
		if len(matches) == 2 {
			return matches[1]
		}
		return ""
	}

	if tag := extractVersion(version); tag != "" {
		return fmt.Sprintf("%s/v%s", ResticURL, tag)
	}

	return ResticURL
}

func openDocs(GOOS string, url string, docType string) {
	_, _ = fmt.Fprintf(stdout, "Opening the %s documentation at %s\n", docType, url)

	var cmd *exec.Cmd

	switch GOOS {
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
