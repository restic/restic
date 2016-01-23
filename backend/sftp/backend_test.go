// DO NOT EDIT, AUTOMATICALLY GENERATED
package sftp_test

import (
	"testing"

	"github.com/restic/restic/backend/test"
)

var SkipMessage string

func TestSftpBackendCreate(t *testing.T) {
	if SkipMessage != "" {
		t.Skip(SkipMessage)
	}
	test.Create(t)
}

func TestSftpBackendOpen(t *testing.T) {
	if SkipMessage != "" {
		t.Skip(SkipMessage)
	}
	test.Open(t)
}

func TestSftpBackendCreateWithConfig(t *testing.T) {
	if SkipMessage != "" {
		t.Skip(SkipMessage)
	}
	test.CreateWithConfig(t)
}

func TestSftpBackendLocation(t *testing.T) {
	if SkipMessage != "" {
		t.Skip(SkipMessage)
	}
	test.Location(t)
}

func TestSftpBackendConfig(t *testing.T) {
	if SkipMessage != "" {
		t.Skip(SkipMessage)
	}
	test.Config(t)
}

func TestSftpBackendGetReader(t *testing.T) {
	if SkipMessage != "" {
		t.Skip(SkipMessage)
	}
	test.GetReader(t)
}

func TestSftpBackendLoad(t *testing.T) {
	if SkipMessage != "" {
		t.Skip(SkipMessage)
	}
	test.Load(t)
}

func TestSftpBackendWrite(t *testing.T) {
	if SkipMessage != "" {
		t.Skip(SkipMessage)
	}
	test.Write(t)
}

func TestSftpBackendGeneric(t *testing.T) {
	if SkipMessage != "" {
		t.Skip(SkipMessage)
	}
	test.Generic(t)
}

func TestSftpBackendDelete(t *testing.T) {
	if SkipMessage != "" {
		t.Skip(SkipMessage)
	}
	test.Delete(t)
}

func TestSftpBackendCleanup(t *testing.T) {
	if SkipMessage != "" {
		t.Skip(SkipMessage)
	}
	test.Cleanup(t)
}
