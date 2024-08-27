package filter

import (
	"testing"
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
			reject := RejectByPattern(patterns, nil)
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
			reject := RejectByInsensitivePattern(patterns, nil)
			res := reject(tc.filename)
			if res != tc.reject {
				t.Fatalf("wrong result for filename %v: want %v, got %v",
					tc.filename, tc.reject, res)
			}
		})
	}
}
