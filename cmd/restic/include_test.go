package main

import (
	"testing"
)

func TestIncludeByPattern(t *testing.T) {
	var tests = []struct {
		filename string
		include  bool
	}{
		{filename: "/home/user/foo.go", include: true},
		{filename: "/home/user/foo.c", include: false},
		{filename: "/home/user/foobar", include: false},
		{filename: "/home/user/foobar/x", include: false},
		{filename: "/home/user/README", include: false},
		{filename: "/home/user/README.md", include: true},
	}

	patterns := []string{"*.go", "README.md"}

	for _, tc := range tests {
		t.Run(tc.filename, func(t *testing.T) {
			includeFunc := includeByPattern(patterns)
			matched, _ := includeFunc(tc.filename)
			if matched != tc.include {
				t.Fatalf("wrong result for filename %v: want %v, got %v",
					tc.filename, tc.include, matched)
			}
		})
	}
}

func TestIncludeByInsensitivePattern(t *testing.T) {
	var tests = []struct {
		filename string
		include  bool
	}{
		{filename: "/home/user/foo.GO", include: true},
		{filename: "/home/user/foo.c", include: false},
		{filename: "/home/user/foobar", include: false},
		{filename: "/home/user/FOObar/x", include: false},
		{filename: "/home/user/README", include: false},
		{filename: "/home/user/readme.MD", include: true},
	}

	patterns := []string{"*.go", "README.md"}

	for _, tc := range tests {
		t.Run(tc.filename, func(t *testing.T) {
			includeFunc := includeByInsensitivePattern(patterns)
			matched, _ := includeFunc(tc.filename)
			if matched != tc.include {
				t.Fatalf("wrong result for filename %v: want %v, got %v",
					tc.filename, tc.include, matched)
			}
		})
	}
}
