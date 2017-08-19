package main

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
)

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
			tempDir, err := ioutil.TempDir("", "restic-test-")
			if err != nil {
				t.Fatalf("could not create temp dir: %v", err)
			}
			defer os.RemoveAll(tempDir)
			foo := filepath.Join(tempDir, "foo")
			err = ioutil.WriteFile(foo, []byte("foo"), 0666)
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
