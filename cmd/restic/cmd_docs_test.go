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
	// Redirect output to capture Printf
	var buf bytes.Buffer
	stdout = &buf

	// Mock the command execution
	originalStart := start
	start = func(name string, arg ...string) *exec.Cmd {
		// Return a harmless 'echo' command to satisfy .Start()
		return exec.Command("echo", arg...)
	}

	// Restore original global variables after test
	defer func() {
		stdout = os.Stdout
		start = originalStart
	}()

	testURL := "https://example.com"
	openDocs(testURL, "user")
	output := buf.String()

	// Verify the console output contains the correct metadata
	if !strings.Contains(output, "Opening the user documentation") {
		t.Errorf("unexpected output message: %q", output)
	}
	if !strings.Contains(output, testURL) {
		t.Errorf("output did not contain the correct URL: %q", output)
	}
}
