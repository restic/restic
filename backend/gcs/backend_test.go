// DO NOT EDIT, AUTOMATICALLY GENERATED
package gcs_test

import (
	"testing"

	"github.com/restic/restic/backend/test"
)

var SkipMessage string

func TestGcsBackendCreate(t *testing.T) {
	if SkipMessage != "" {
		t.Skip(SkipMessage)
	}
	test.TestCreate(t)
}

func TestGcsBackendOpen(t *testing.T) {
	if SkipMessage != "" {
		t.Skip(SkipMessage)
	}
	test.TestOpen(t)
}

func TestGcsBackendCreateWithConfig(t *testing.T) {
	if SkipMessage != "" {
		t.Skip(SkipMessage)
	}
	test.TestCreateWithConfig(t)
}

func TestGcsBackendLocation(t *testing.T) {
	if SkipMessage != "" {
		t.Skip(SkipMessage)
	}
	test.TestLocation(t)
}

func TestGcsBackendConfig(t *testing.T) {
	if SkipMessage != "" {
		t.Skip(SkipMessage)
	}
	test.TestConfig(t)
}

func TestGcsBackendLoad(t *testing.T) {
	if SkipMessage != "" {
		t.Skip(SkipMessage)
	}
	test.TestLoad(t)
}

func TestGcsBackendSave(t *testing.T) {
	if SkipMessage != "" {
		t.Skip(SkipMessage)
	}
	test.TestSave(t)
}

func TestGcsBackendSaveFilenames(t *testing.T) {
	if SkipMessage != "" {
		t.Skip(SkipMessage)
	}
	test.TestSaveFilenames(t)
}

func TestGcsBackendBackend(t *testing.T) {
	if SkipMessage != "" {
		t.Skip(SkipMessage)
	}
	test.TestBackend(t)
}

func TestGcsBackendDelete(t *testing.T) {
	if SkipMessage != "" {
		t.Skip(SkipMessage)
	}
	test.TestDelete(t)
}

func TestGcsBackendCleanup(t *testing.T) {
	if SkipMessage != "" {
		t.Skip(SkipMessage)
	}
	test.TestCleanup(t)
}
