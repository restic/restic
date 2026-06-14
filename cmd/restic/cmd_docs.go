package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"github.com/restic/restic/internal/global"
)

const (
	GOOS          string = runtime.GOOS
	ResticDocsURL string = "https://restic.readthedocs.io/en"
)

type execFn func(name string, arg ...string) *exec.Cmd

var (
	stdout       io.Writer      = os.Stdout
	start        execFn         = exec.Command
	versionRegex *regexp.Regexp = regexp.MustCompile(`^(\d+\.\d+\.\d+)`)
)

// newDocsCommand is the `docs` subcommand entry point,
// using `restic docs` or `restic docs user` or `restic docs dev`.
// It opens the respective documetation in your chosen default browser.
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
			docsURL := fmt.Sprintf("%s/stable", ResticDocsURL)
			openDocs(GOOS, docsURL, "user")
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "dev",
		Short: "Show the developer documentation",
		Run: func(_ *cobra.Command, _ []string) {
			docsURL := fmt.Sprintf("%s/latest", ResticDocsURL)
			openDocs(GOOS, docsURL, "developer")
		},
	})

	return cmd
}

// docsURLForVersion is a function that returns a the URL documentation as a string.
// it takes a version string as a "v1.2.3" as an input.
func docsURLForVersion(version string) string {
	// Safe default fallback for empty/unknown versions
	if version == "" || version == "unknown" {
		return fmt.Sprintf("%s/stable", ResticDocsURL)
	}

	// Route development builds / local compiled binaries directly to bleeding edge docs
	if strings.Contains(version, "dev") || strings.Contains(version, "compiled") {
		return fmt.Sprintf("%s/latest", ResticDocsURL)
	}

	// Match strict tag releases (e.g., exact matches like "0.18.1")
	matches := versionRegex.FindStringSubmatch(version)
	if len(matches) == 2 {
		return fmt.Sprintf("%s/v%s", ResticDocsURL, matches[1])
	}

	// Return the stable docs if all checks fail
	return fmt.Sprintf("%s/stable", ResticDocsURL)
}

// openDocs is a function that takes in the current operating system platform, documentation url and its type.
// It basically opens the documentation in your chosen default browser.
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
		log.Fatalf("Failed to open browser: %v", err)
	}
}
