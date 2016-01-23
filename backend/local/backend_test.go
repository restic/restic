// DO NOT EDIT, AUTOMATICALLY GENERATED
package local_test

import (
	"testing"

	"github.com/restic/restic/backend/test"
)

var SkipMessage string

func TestLocalBackendCreate(t *testing.T) {
	if SkipMessage != "" {
		t.Skip(SkipMessage)
	}
	test.Create(t)
}

func TestLocalBackendOpen(t *testing.T) {
	if SkipMessage != "" {
		t.Skip(SkipMessage)
	}
	test.Open(t)
}

func TestLocalBackendCreateWithConfig(t *testing.T) {
	if SkipMessage != "" {
		t.Skip(SkipMessage)
	}
	test.CreateWithConfig(t)
}

func TestLocalBackendLocation(t *testing.T) {
	if SkipMessage != "" {
		t.Skip(SkipMessage)
	}
	test.Location(t)
}

func TestLocalBackendConfig(t *testing.T) {
	if SkipMessage != "" {
		t.Skip(SkipMessage)
	}
	test.Config(t)
}

func TestLocalBackendGetReader(t *testing.T) {
	if SkipMessage != "" {
		t.Skip(SkipMessage)
	}
	test.GetReader(t)
}

func TestLocalBackendLoad(t *testing.T) {
	if SkipMessage != "" {
		t.Skip(SkipMessage)
	}
	test.Load(t)
}

func TestLocalBackendWrite(t *testing.T) {
	if SkipMessage != "" {
		t.Skip(SkipMessage)
	}
	test.Write(t)
}

func TestLocalBackendGeneric(t *testing.T) {
	if SkipMessage != "" {
		t.Skip(SkipMessage)
	}
	test.Generic(t)
}

func TestLocalBackendDelete(t *testing.T) {
	if SkipMessage != "" {
		t.Skip(SkipMessage)
	}
	test.Delete(t)
}

func TestLocalBackendCleanup(t *testing.T) {
	if SkipMessage != "" {
		t.Skip(SkipMessage)
	}
	test.Cleanup(t)
}
