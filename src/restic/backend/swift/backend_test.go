// DO NOT EDIT, AUTOMATICALLY GENERATED
package swift_test

import (
	"testing"

	"restic/backend/test"
)

var SkipMessage string

func TestSwiftBackendCreate(t *testing.T) {
	if SkipMessage != "" {
		t.Skip(SkipMessage)
	}
	test.TestCreate(t)
}

func TestSwiftBackendOpen(t *testing.T) {
	if SkipMessage != "" {
		t.Skip(SkipMessage)
	}
	test.TestOpen(t)
}

func TestSwiftBackendCreateWithConfig(t *testing.T) {
	if SkipMessage != "" {
		t.Skip(SkipMessage)
	}
	test.TestCreateWithConfig(t)
}

func TestSwiftBackendLocation(t *testing.T) {
	if SkipMessage != "" {
		t.Skip(SkipMessage)
	}
	test.TestLocation(t)
}

func TestSwiftBackendConfig(t *testing.T) {
	if SkipMessage != "" {
		t.Skip(SkipMessage)
	}
	test.TestConfig(t)
}

func TestSwiftBackendLoad(t *testing.T) {
	if SkipMessage != "" {
		t.Skip(SkipMessage)
	}
	test.TestLoad(t)
}

func TestSwiftBackendSave(t *testing.T) {
	if SkipMessage != "" {
		t.Skip(SkipMessage)
	}
	test.TestSave(t)
}

func TestSwiftBackendSaveFilenames(t *testing.T) {
	if SkipMessage != "" {
		t.Skip(SkipMessage)
	}
	test.TestSaveFilenames(t)
}

func TestSwiftBackendBackend(t *testing.T) {
	if SkipMessage != "" {
		t.Skip(SkipMessage)
	}
	test.TestBackend(t)
}

func TestSwiftBackendDelete(t *testing.T) {
	if SkipMessage != "" {
		t.Skip(SkipMessage)
	}
	test.TestDelete(t)
}

func TestSwiftBackendCleanup(t *testing.T) {
	if SkipMessage != "" {
		t.Skip(SkipMessage)
	}
	test.TestCleanup(t)
}
