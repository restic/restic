package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/restic/restic/internal/global"
)

func TestNewDocsCommand(t *testing.T) {
	cmd := newDocsCommand(&global.Options{})

	if cmd.Use != "docs" {
		t.Errorf("expected command Use 'docs' got %q", cmd.Use)
	}

	subcommands := []struct {
		name  string
		short string
	}{
		{"user", "Show the user documentation"},
		{"dev", "Show the developer documentation"},
	}

	for _, sc := range subcommands {
		found := false
		for _, sub := range cmd.Commands() {
			if sub.Name() == sc.name {
				found = true
				if sub.Short != sc.short {
					t.Errorf("expected short description %q for %q, got %q", sc.short, sc.name, sub.Short)
				}
			}
		}
		if !found {
			t.Errorf("expected subcommand %q not found", sc.name)
		}
	}
}

func TestDocsURLForVersion(t *testing.T) {
	// Dynamically build the expected URLs using the package's base constant
	stableURL := fmt.Sprintf("%s/stable", ResticURL)
	latestURL := fmt.Sprintf("%s/latest", ResticURL)
	tagURL := func(tag string) string {
		return fmt.Sprintf("%s/v%s", ResticURL, tag)
	}

	tests := []struct {
		name    string
		version string
		want    string
	}{
		// --- 1. Stable Tag Releases ---
		{
			name:    "Exact Release Tag",
			version: "0.18.1",
			want:    tagURL("0.18.1"),
		},
		{
			name:    "Release Tag with Patch",
			version: "1.23.4",
			want:    tagURL("1.23.4"),
		},

		// --- 2. Development & Bleeding Edge Builds ---
		{
			name:    "Dev Build with Suffix",
			version: "0.18.1-dev",
			want:    latestURL,
		},
		{
			name:    "Manually Compiled Binary",
			version: "0.18.1 (compiled manually)",
			want:    latestURL,
		},
		{
			name:    "Pure Dev Keyword",
			version: "dev",
			want:    latestURL,
		},

		// --- 3. Fallbacks & Unknown States ---
		{
			name:    "Explicit Unknown Keyword",
			version: "unknown",
			want:    stableURL,
		},
		{
			name:    "Empty String Fallback",
			version: "",
			want:    stableURL,
		},
		{
			name:    "Malformed Version Fallback",
			version: "my-custom-version-string",
			want:    stableURL,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := docsURLForVersion(tt.version); got != tt.want {
				t.Errorf("docsURLForVersion(%q) = %q, want %q", tt.version, got, tt.want)
			}
		})
	}
}

func TestOpenDocs(t *testing.T) {
	stableURL := fmt.Sprintf("%s/stable", ResticURL)
	latestURL := fmt.Sprintf("%s/latest", ResticURL)
	tagURL := func(tag string) string {
		return fmt.Sprintf("%s/v%s", ResticURL, tag)
	}

	tests := []struct {
		name    string
		goos    string
		url     string
		docType string
		wantBin string
		wantArg string
	}{
		{"Linux version", "linux", tagURL("v0.18.1"), "user", "xdg-open", tagURL("v0.18.1")},
		{"Linux User", "linux", stableURL, "user", "xdg-open", stableURL},
		{"Linux Dev", "linux", latestURL, "developer", "xdg-open", latestURL},

		{"Mac Version", "darwin", tagURL("v0.18.1"), "user", "open", tagURL("v0.18.1")},
		{"Mac User", "darwin", stableURL, "user", "open", stableURL},
		{"Mac Dev", "darwin", latestURL, "developer", "open", latestURL},

		{"Windows Version", "windows", tagURL("v0.18.1"), "user", "rundll32", "url.dll,FileProtocolHandler"},
		{"Windows User", "windows", stableURL, "user", "rundll32", "url.dll,FileProtocolHandler"},
		{"Windows Dev", "windows", latestURL, "developer", "rundll32", "url.dll,FileProtocolHandler"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			stdout = &buf // Capture console output

			var capturedBin string
			var capturedArgs []string

			// Mock the execution
			originalStart := start
			start = func(name string, arg ...string) *exec.Cmd {
				capturedBin = name
				capturedArgs = arg
				return exec.Command("echo")
			}

			// Cleanup the global state
			defer func() {
				stdout = os.Stdout
				start = originalStart
			}()

			openDocs(tt.goos, tt.url, tt.docType)

			// Test 1. Verify Command Binary
			if capturedBin != tt.wantBin {
				t.Errorf("Binary mismatch: expected %q, got %q", tt.wantBin, capturedBin)
			}

			// Test 2. Verify Command Arguments
			argsJoined := strings.Join(capturedArgs, " ")
			if !strings.Contains(argsJoined, tt.wantArg) {
				t.Errorf("Args mismatch: expected %q, got %q", tt.wantArg, argsJoined)
			}

			// Test 3. Verify Console Output Message
			output := buf.String()
			if !strings.Contains(output, tt.url) || !strings.Contains(output, tt.docType) {
				t.Errorf("Console output mismatch. Got: %q", output)
			}
		})
	}
}
