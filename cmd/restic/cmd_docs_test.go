package main

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestNewDocsCommand(t *testing.T) {
	cmd := newDocsCommand()

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

func TestOpenDocs(t *testing.T) {
	tests := []struct {
		name    string
		goos    string
		url     string
		docType string
		wantBin string
		wantArg string
	}{
		{"Linux User", "linux", ResticDocsURL, "user", "xdg-open", ResticDocsURL},
		{"Linux Dev", "linux", ResticDevDocsURL, "developer", "xdg-open", ResticDevDocsURL},
		{"Mac User", "darwin", ResticDocsURL, "user", "open", ResticDocsURL},
		{"Mac Dev", "darwin", ResticDevDocsURL, "developer", "open", ResticDevDocsURL},
		{"Windows User", "windows", ResticDocsURL, "user", "rundll32", "url.dll,FileProtocolHandler"},
		{"Windows Dev", "windows", ResticDevDocsURL, "developer", "rundll32", "url.dll,FileProtocolHandler"},
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
