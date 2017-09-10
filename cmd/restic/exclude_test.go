package main

import (
	"io/ioutil"
	"path/filepath"
	"testing"

	"github.com/restic/restic/internal/test"
)

func TestRejectByPattern(t *testing.T) {
	var tests = []struct {
		filename string
		reject   bool
	}{
		{filename: "/home/user/foo.go", reject: true},
		{filename: "/home/user/foo.c", reject: false},
		{filename: "/home/user/foobar", reject: false},
		{filename: "/home/user/foobar/x", reject: true},
		{filename: "/home/user/README", reject: false},
		{filename: "/home/user/README.md", reject: true},
	}

	patterns := []string{"*.go", "README.md", "/home/user/foobar/*"}

	for _, tc := range tests {
		t.Run("", func(t *testing.T) {
			reject := rejectByPattern(patterns)
			res := reject(tc.filename, nil)
			if res != tc.reject {
				t.Fatalf("wrong result for filename %v: want %v, got %v",
					tc.filename, tc.reject, res)
			}
		})
	}
}

func TestIsExcludedByFile(t *testing.T) {
	const (
		tagFilename = "CACHEDIR.TAG"
		header      = "Signature: 8a477f597d28d172789f06886806bc55"
	)
	tests := []struct {
		name    string
		tagFile string
		content string
		want    bool
	}{
		{"NoTagfile", "", "", false},
		{"EmptyTagfile", tagFilename, "", true},
		{"UnnamedTagFile", "", header, false},
		{"WrongTagFile", "notatagfile", header, false},
		{"IncorrectSig", tagFilename, header[1:], false},
		{"ValidSig", tagFilename, header, true},
		{"ValidPlusStuff", tagFilename, header + "foo", true},
		{"ValidPlusNewlineAndStuff", tagFilename, header + "\nbar", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tempDir, cleanup := test.TempDir(t)
			defer cleanup()

			foo := filepath.Join(tempDir, "foo")
			err := ioutil.WriteFile(foo, []byte("foo"), 0666)
			if err != nil {
				t.Fatalf("could not write file: %v", err)
			}
			if tc.tagFile != "" {
				tagFile := filepath.Join(tempDir, tc.tagFile)
				err = ioutil.WriteFile(tagFile, []byte(tc.content), 0666)
				if err != nil {
					t.Fatalf("could not write tagfile: %v", err)
				}
			}
			h := header
			if tc.content == "" {
				h = ""
			}
			if got := isExcludedByFile(foo, tagFilename, h); tc.want != got {
				t.Fatalf("expected %v, got %v", tc.want, got)
			}
		})
	}
}
