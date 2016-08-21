// DO NOT EDIT, AUTOMATICALLY GENERATED
package s3_test

import (
	"testing"

	"restic/backend/test"
)

var SkipMessage string

func TestS3BackendCreate(t *testing.T) {
	if SkipMessage != "" {
		t.Skip(SkipMessage)
	}
	test.TestCreate(t)
}

func TestS3BackendOpen(t *testing.T) {
	if SkipMessage != "" {
		t.Skip(SkipMessage)
	}
	test.TestOpen(t)
}

func TestS3BackendCreateWithConfig(t *testing.T) {
	if SkipMessage != "" {
		t.Skip(SkipMessage)
	}
	test.TestCreateWithConfig(t)
}

func TestS3BackendLocation(t *testing.T) {
	if SkipMessage != "" {
		t.Skip(SkipMessage)
	}
	test.TestLocation(t)
}

func TestS3BackendConfig(t *testing.T) {
	if SkipMessage != "" {
		t.Skip(SkipMessage)
	}
	test.TestConfig(t)
}

func TestS3BackendLoad(t *testing.T) {
	if SkipMessage != "" {
		t.Skip(SkipMessage)
	}
	test.TestLoad(t)
}

func TestS3BackendLoadNegativeOffset(t *testing.T) {
	if SkipMessage != "" {
		t.Skip(SkipMessage)
	}
	test.TestLoadNegativeOffset(t)
}

func TestS3BackendSave(t *testing.T) {
	if SkipMessage != "" {
		t.Skip(SkipMessage)
	}
	test.TestSave(t)
}

func TestS3BackendSaveFilenames(t *testing.T) {
	if SkipMessage != "" {
		t.Skip(SkipMessage)
	}
	test.TestSaveFilenames(t)
}

func TestS3BackendBackend(t *testing.T) {
	if SkipMessage != "" {
		t.Skip(SkipMessage)
	}
	test.TestBackend(t)
}

func TestS3BackendDelete(t *testing.T) {
	if SkipMessage != "" {
		t.Skip(SkipMessage)
	}
	test.TestDelete(t)
}

func TestS3BackendCleanup(t *testing.T) {
	if SkipMessage != "" {
		t.Skip(SkipMessage)
	}
	test.TestCleanup(t)
}
