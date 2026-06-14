package main

import (
	"bytes"
	"context"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/restic/restic/internal/global"
	"github.com/restic/restic/internal/ui/termstatus"
)

func newTestTerm() (*termstatus.Terminal, func()) {
	ctx, cancel := context.WithCancel(context.Background())
	term := termstatus.New(io.NopCloser(bytes.NewReader(nil)), io.Discard, io.Discard, true)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		term.Run(ctx)
	}()

	return term, func() {
		cancel()
		wg.Wait()
	}
}

func TestTestPatternMatch(t *testing.T) {
	for _, test := range []struct {
		name            string
		pattern         string
		path            string
		caseInsensitive bool
		wantMatch       bool
		wantErr         bool
	}{
		{
			name:      "simple wildcard match",
			pattern:   "*.go",
			path:      "/home/user/main.go",
			wantMatch: true,
		},
		{
			name:      "simple wildcard no match",
			pattern:   "*.txt",
			path:      "/home/user/main.go",
			wantMatch: false,
		},
		{
			name:      "exact filename match",
			pattern:   "main.go",
			path:      "main.go",
			wantMatch: true,
		},
		{
			name:            "case insensitive match",
			pattern:         "*.GO",
			path:            "/home/user/main.go",
			caseInsensitive: true,
			wantMatch:       true,
		},
		{
			name:            "case insensitive no match",
			pattern:         "*.txt",
			path:            "/home/user/main.go",
			caseInsensitive: true,
			wantMatch:       false,
		},
		{
			name:      "recursive wildcard match",
			pattern:   "**/.git/**",
			path:      "/home/user/project/.git/config",
			wantMatch: true,
		},
		{
			name:      "recursive wildcard child match",
			pattern:   "**/.git/**",
			path:      "/home/user/project/.git",
			wantMatch: true,
		},
		{
			name:      "absolute pattern match",
			pattern:   "/home/user/*.txt",
			path:      "/home/user/readme.txt",
			wantMatch: true,
		},
		{
			name:      "absolute pattern no match different dir",
			pattern:   "/home/user/*.txt",
			path:      "/home/other/readme.txt",
			wantMatch: false,
		},
		{
			name:      "single character wildcard",
			pattern:   "file.?o",
			path:      "/path/file.go",
			wantMatch: true,
		},
		{
			name:      "question mark no match",
			pattern:   "file.?o",
			path:      "/path/file.py",
			wantMatch: false,
		},
		{
			name:      "character class match",
			pattern:   "file.[gx]o",
			path:      "/path/file.go",
			wantMatch: true,
		},
		{
			name:      "character class no match",
			pattern:   "file.[ab]o",
			path:      "/path/file.go",
			wantMatch: false,
		},
		{
			name:      "directory wildcard match",
			pattern:   "/home/*/main.go",
			path:      "/home/user/main.go",
			wantMatch: true,
		},
		{
			name:      "directory wildcard no match subdir",
			pattern:   "/home/*/main.go",
			path:      "/home/user/sub/main.go",
			wantMatch: false,
		},
		{
			name:      "directory wildcard multi-level",
			pattern:   "/home/*/*/main.go",
			path:      "/home/user/sub/main.go",
			wantMatch: true,
		},
		{
			name:      "invalid pattern",
			pattern:   "[",
			path:      "/home/user/main.go",
			wantErr:   true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			term, cleanup := newTestTerm()
			defer cleanup()

			gopts := global.Options{
				Term: term,
			}

			opts := TestPatternOptions{
				CaseInsensitive: test.caseInsensitive,
			}

			args := []string{test.pattern, test.path}
			err := runTestPattern(opts, gopts, args)

			if test.wantErr {
				if err == nil {
					t.Fatal("expected error but got none")
				}
				return
			}

			if test.wantMatch {
				if err != nil {
					t.Fatalf("expected match but got error: %v", err)
				}
			} else {
				if err == nil {
					t.Fatal("expected no match but got error")
				}
				if !strings.Contains(err.Error(), "pattern did not match") {
					t.Fatalf("expected 'pattern did not match' error, got: %v", err)
				}
			}
		})
	}
}

func TestTestPatternWrongArgCount(t *testing.T) {
	term, cleanup := newTestTerm()
	defer cleanup()

	gopts := global.Options{
		Term: term,
	}

	opts := TestPatternOptions{}

	err := runTestPattern(opts, gopts, []string{})
	if err == nil || !strings.Contains(err.Error(), "wrong number of arguments") {
		t.Fatalf("expected 'wrong number of arguments' error, got: %v", err)
	}

	err = runTestPattern(opts, gopts, []string{"*.go"})
	if err == nil || !strings.Contains(err.Error(), "wrong number of arguments") {
		t.Fatalf("expected 'wrong number of arguments' error, got: %v", err)
	}

	err = runTestPattern(opts, gopts, []string{"*.go", "/path", "extra"})
	if err == nil || !strings.Contains(err.Error(), "wrong number of arguments") {
		t.Fatalf("expected 'wrong number of arguments' error, got: %v", err)
	}
}
