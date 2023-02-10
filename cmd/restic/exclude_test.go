package main

import (
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

func TestRejectByInsensitivePattern(t *testing.T) {
	var tests = []struct {
		filename string
		reject   bool
	}{
		{filename: "/home/user/foo.GO", reject: true},
		{filename: "/home/user/foo.c", reject: false},
		{filename: "/home/user/foobar", reject: false},
		{filename: "/home/user/FOObar/x", reject: true},
		{filename: "/home/user/README", reject: false},
		{filename: "/home/user/readme.md", reject: true},
	}

	patterns := []string{"*.go", "README.md", "/home/user/foobar/*"}

	for _, tc := range tests {
		t.Run("", func(t *testing.T) {
			reject := rejectByInsensitivePattern(patterns)
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
			tempDir := test.TempDir(t)

			foo := filepath.Join(tempDir, "foo")
			err := os.WriteFile(foo, []byte("foo"), 0666)
			if err != nil {
				t.Fatalf("could not write file: %v", err)
			}
			if tc.tagFile != "" {
				tagFile := filepath.Join(tempDir, tc.tagFile)
				err = os.WriteFile(tagFile, []byte(tc.content), 0666)
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
	tempDir := test.TempDir(t)

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
		errs = append(errs, os.WriteFile(p, []byte(f.path), 0600))
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

func TestParseSizeStr(t *testing.T) {
	sizeStrTests := []struct {
		in       string
		expected int64
	}{
		{"1024", 1024},
		{"1024b", 1024},
		{"1024B", 1024},
		{"1k", 1024},
		{"100k", 102400},
		{"100K", 102400},
		{"10M", 10485760},
		{"100m", 104857600},
		{"20G", 21474836480},
		{"10g", 10737418240},
		{"2T", 2199023255552},
		{"2t", 2199023255552},
	}

	for _, tt := range sizeStrTests {
		actual, err := parseSizeStr(tt.in)
		test.OK(t, err)

		if actual != tt.expected {
			t.Errorf("parseSizeStr(%s) = %d; expected %d", tt.in, actual, tt.expected)
		}
	}
}

func TestParseInvalidSizeStr(t *testing.T) {
	invalidSizes := []string{
		"",
		" ",
		"foobar",
		"zzz",
	}

	for _, s := range invalidSizes {
		v, err := parseSizeStr(s)
		if err == nil {
			t.Errorf("wanted error for invalid value %q, got nil", s)
		}
		if v != 0 {
			t.Errorf("wanted zero for invalid value %q, got: %v", s, v)
		}
	}
}

// TestIsExcludedByFileSize is for testing the instance of
// --exclude-larger-than parameters
func TestIsExcludedByFileSize(t *testing.T) {
	tempDir := test.TempDir(t)

	// Max size of file is set to be 1k
	maxSizeStr := "1k"

	// Create some files in a temporary directory.
	// Files in UPPERCASE will be used as exclusion triggers later on.
	// We will test the inclusion later, so we add the expected value as
	// a bool.
	files := []struct {
		path string
		size int64
		incl bool
	}{
		{"42", 100, true},

		// everything in foodir except the FOOLARGE tagfile
		// should not be included.
		{"foodir/FOOLARGE", 2048, false},
		{"foodir/foo", 1002, true},
		{"foodir/foosub/underfoo", 100, true},

		// everything in bardir except the BARLARGE tagfile
		// should not be included.
		{"bardir/BARLARGE", 1030, false},
		{"bardir/bar", 1000, true},
		{"bardir/barsub/underbar", 500, true},

		// everything in bazdir should be included.
		{"bazdir/baz", 100, true},
		{"bazdir/bazsub/underbaz", 200, true},
	}
	var errs []error
	for _, f := range files {
		// create directories first, then the file
		p := filepath.Join(tempDir, filepath.FromSlash(f.path))
		errs = append(errs, os.MkdirAll(filepath.Dir(p), 0700))
		file, err := os.OpenFile(p, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
		errs = append(errs, err)
		if err == nil {
			// create a file with given size
			errs = append(errs, file.Truncate(f.size))
		}
		errs = append(errs, file.Close())
	}
	test.OKs(t, errs) // see if anything went wrong during the creation

	// create rejection function
	sizeExclude, _ := rejectBySize(maxSizeStr)

	// To mock the archiver scanning walk, we create filepath.WalkFn
	// that tests against the two rejection functions and stores
	// the result in a map against we can test later.
	m := make(map[string]bool)
	walk := func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		excluded := sizeExclude(p, fi)
		// the log message helps debugging in case the test fails
		t.Logf("%q: dir:%t; size:%d; excluded:%v", p, fi.IsDir(), fi.Size(), excluded)
		m[p] = !excluded
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

func TestDeviceMap(t *testing.T) {
	deviceMap := DeviceMap{
		filepath.FromSlash("/"):          1,
		filepath.FromSlash("/usr/local"): 5,
	}

	var tests = []struct {
		item     string
		deviceID uint64
		allowed  bool
	}{
		{"/root", 1, true},
		{"/usr", 1, true},

		{"/proc", 2, false},
		{"/proc/1234", 2, false},

		{"/usr", 3, false},
		{"/usr/share", 3, false},

		{"/usr/local", 5, true},
		{"/usr/local/foobar", 5, true},

		{"/usr/local/foobar/submount", 23, false},
		{"/usr/local/foobar/submount/file", 23, false},

		{"/usr/local/foobar/outhersubmount", 1, false},
		{"/usr/local/foobar/outhersubmount/otherfile", 1, false},
	}

	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			res, err := deviceMap.IsAllowed(filepath.FromSlash(test.item), test.deviceID)
			if err != nil {
				t.Fatal(err)
			}

			if res != test.allowed {
				t.Fatalf("wrong result returned by IsAllowed(%v): want %v, got %v", test.item, test.allowed, res)
			}
		})
	}
}
