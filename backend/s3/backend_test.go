// DO NOT EDIT, AUTOMATICALLY GENERATED
package s3_test

import (
	"testing"

	"github.com/restic/restic/backend/test"
)

var SkipMessage string

func TestS3BackendCreate(t *testing.T) {
	if SkipMessage != "" {
		t.Skip(SkipMessage)
	}
	test.Create(t)
}

func TestS3BackendOpen(t *testing.T) {
	if SkipMessage != "" {
		t.Skip(SkipMessage)
	}
	test.Open(t)
}

func TestS3BackendCreateWithConfig(t *testing.T) {
	if SkipMessage != "" {
		t.Skip(SkipMessage)
	}
	test.CreateWithConfig(t)
}

func TestS3BackendLocation(t *testing.T) {
	if SkipMessage != "" {
		t.Skip(SkipMessage)
	}
	test.Location(t)
}

func TestS3BackendConfig(t *testing.T) {
	if SkipMessage != "" {
		t.Skip(SkipMessage)
	}
	test.Config(t)
}

func TestS3BackendGetReader(t *testing.T) {
	if SkipMessage != "" {
		t.Skip(SkipMessage)
	}
	test.GetReader(t)
}

func TestS3BackendLoad(t *testing.T) {
	if SkipMessage != "" {
		t.Skip(SkipMessage)
	}
	test.Load(t)
}

func TestS3BackendWrite(t *testing.T) {
	if SkipMessage != "" {
		t.Skip(SkipMessage)
	}
	test.Write(t)
}

func TestS3BackendGeneric(t *testing.T) {
	if SkipMessage != "" {
		t.Skip(SkipMessage)
	}
	test.Generic(t)
}

func TestS3BackendDelete(t *testing.T) {
	if SkipMessage != "" {
		t.Skip(SkipMessage)
	}
	test.Delete(t)
}

func TestS3BackendCleanup(t *testing.T) {
	if SkipMessage != "" {
		t.Skip(SkipMessage)
	}
	test.Cleanup(t)
}
