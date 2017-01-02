package b2_test

import (
	"testing"

	"restic/backend/test"
)

var SkipMessage string

func TestB2BackendCreate(t *testing.T) {
	if SkipMessage != "" {
		t.Skip(SkipMessage)
	}
	test.TestCreate(t)
}

func TestB2BackendOpen(t *testing.T) {
	if SkipMessage != "" {
		t.Skip(SkipMessage)
	}
	test.TestOpen(t)
}

func TestB2BackendCreateWithConfig(t *testing.T) {
	if SkipMessage != "" {
		t.Skip(SkipMessage)
	}
	test.TestCreateWithConfig(t)
}

func TestB2BackendLocation(t *testing.T) {
	if SkipMessage != "" {
		t.Skip(SkipMessage)
	}
	test.TestLocation(t)
}

func TestB2BackendConfig(t *testing.T) {
	if SkipMessage != "" {
		t.Skip(SkipMessage)
	}
	test.TestConfig(t)
}

func TestB2BackendLoad(t *testing.T) {
	if SkipMessage != "" {
		t.Skip(SkipMessage)
	}
	test.TestLoad(t)
}

func TestB2BackendLoadNegativeOffset(t *testing.T) {
	if SkipMessage != "" {
		t.Skip(SkipMessage)
	}
	test.TestLoadNegativeOffset(t)
}

func TestB2BackendSave(t *testing.T) {
	if SkipMessage != "" {
		t.Skip(SkipMessage)
	}
	test.TestSave(t)
}

func TestB2BackendSaveFilenames(t *testing.T) {
	if SkipMessage != "" {
		t.Skip(SkipMessage)
	}
	test.TestSaveFilenames(t)
}

func TestB2BackendBackend(t *testing.T) {
	if SkipMessage != "" {
		t.Skip(SkipMessage)
	}
	test.TestBackend(t)
}

func TestB2BackendDelete(t *testing.T) {
	if SkipMessage != "" {
		t.Skip(SkipMessage)
	}
	test.TestDelete(t)
}

func TestB2BackendCleanup(t *testing.T) {
	if SkipMessage != "" {
		t.Skip(SkipMessage)
	}
	test.TestCleanup(t)
}
