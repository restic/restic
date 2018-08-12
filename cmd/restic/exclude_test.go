package main

import (
	"io/ioutil"
	"os"
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
			res := reject(tc.filename)
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
			if got := isExcludedByFile(foo, tagFilename, h, nil); tc.want != got {
				t.Fatalf("expected %v, got %v", tc.want, got)
			}
		})
	}
}

// TestMultipleIsExcludedByFile is for testing that multiple instances of
// the --exclude-if-present parameter (or the shortcut --exclude-caches do not
// cancel each other out. It was initially written to demonstrate a bug in
// rejectIfPresent.
func TestMultipleIsExcludedByFile(t *testing.T) {
	tempDir, cleanup := test.TempDir(t)
	defer cleanup()

	// Create some files in a temporary directory.
	// Files in UPPERCASE will be used as exclusion triggers later on.
	// We will test the inclusion later, so we add the expected value as
	// a bool.
	files := []struct {
		path string
		incl bool
	}{
		{"42", true},

		// everything in foodir except the NOFOO tagfile
		// should not be included.
		{"foodir/NOFOO", true},
		{"foodir/foo", false},
		{"foodir/foosub/underfoo", false},

		// everything in bardir except the NOBAR tagfile
		// should not be included.
		{"bardir/NOBAR", true},
		{"bardir/bar", false},
		{"bardir/barsub/underbar", false},

		// everything in bazdir should be included.
		{"bazdir/baz", true},
		{"bazdir/bazsub/underbaz", true},
	}
	var errs []error
	for _, f := range files {
		// create directories first, then the file
		p := filepath.Join(tempDir, filepath.FromSlash(f.path))
		errs = append(errs, os.MkdirAll(filepath.Dir(p), 0700))
		errs = append(errs, ioutil.WriteFile(p, []byte(f.path), 0600))
	}
	test.OKs(t, errs) // see if anything went wrong during the creation

	// create two rejection functions, one that tests for the NOFOO file
	// and one for the NOBAR file
	fooExclude, _ := rejectIfPresent("NOFOO")
	barExclude, _ := rejectIfPresent("NOBAR")

	// To mock the archiver scanning walk, we create filepath.WalkFn
	// that tests against the two rejection functions and stores
	// the result in a map against we can test later.
	m := make(map[string]bool)
	walk := func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		excludedByFoo := fooExclude(p)
		excludedByBar := barExclude(p)
		excluded := excludedByFoo || excludedByBar
		// the log message helps debugging in case the test fails
		t.Logf("%q: %v || %v = %v", p, excludedByFoo, excludedByBar, excluded)
		m[p] = !excluded
		if excluded {
			return filepath.SkipDir
		}
		return nil
	}
	// walk through the temporary file and check the error
	test.OK(t, filepath.Walk(tempDir, walk))

	// compare whether the walk gave the expected values for the test cases
	for _, f := range files {
		p := filepath.Join(tempDir, filepath.FromSlash(f.path))
		if m[p] != f.incl {
			t.Errorf("inclusion status of %s is wrong: want %v, got %v", f.path, f.incl, m[p])
		}
	}
}
