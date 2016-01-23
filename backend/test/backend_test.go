// DO NOT EDIT, AUTOMATICALLY GENERATED
package test_test

import (
	"testing"

	"github.com/restic/restic/backend/test"
)

var SkipMessage string

func TestTestBackendCreate(t *testing.T) {
	if SkipMessage != "" {
		t.Skip(SkipMessage)
	}
	test.Create(t)
}

func TestTestBackendOpen(t *testing.T) {
	if SkipMessage != "" {
		t.Skip(SkipMessage)
	}
	test.Open(t)
}

func TestTestBackendCreateWithConfig(t *testing.T) {
	if SkipMessage != "" {
		t.Skip(SkipMessage)
	}
	test.CreateWithConfig(t)
}

func TestTestBackendLocation(t *testing.T) {
	if SkipMessage != "" {
		t.Skip(SkipMessage)
	}
	test.Location(t)
}

func TestTestBackendConfig(t *testing.T) {
	if SkipMessage != "" {
		t.Skip(SkipMessage)
	}
	test.Config(t)
}

func TestTestBackendGetReader(t *testing.T) {
	if SkipMessage != "" {
		t.Skip(SkipMessage)
	}
	test.GetReader(t)
}

func TestTestBackendLoad(t *testing.T) {
	if SkipMessage != "" {
		t.Skip(SkipMessage)
	}
	test.Load(t)
}

func TestTestBackendWrite(t *testing.T) {
	if SkipMessage != "" {
		t.Skip(SkipMessage)
	}
	test.Write(t)
}

func TestTestBackendGeneric(t *testing.T) {
	if SkipMessage != "" {
		t.Skip(SkipMessage)
	}
	test.Generic(t)
}

func TestTestBackendDelete(t *testing.T) {
	if SkipMessage != "" {
		t.Skip(SkipMessage)
	}
	test.Delete(t)
}

func TestTestBackendCleanup(t *testing.T) {
	if SkipMessage != "" {
		t.Skip(SkipMessage)
	}
	test.Cleanup(t)
}
