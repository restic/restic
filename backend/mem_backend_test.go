// DO NOT EDIT, AUTOMATICALLY GENERATED
package backend_test

import (
	"testing"

	"github.com/restic/restic/backend/test"
)

func TestMemBackendCreate(t *testing.T)           { test.Create(t) }
func TestMemBackendOpen(t *testing.T)             { test.Open(t) }
func TestMemBackendCreateWithConfig(t *testing.T) { test.CreateWithConfig(t) }
func TestMemBackendLocation(t *testing.T)         { test.Location(t) }
func TestMemBackendConfig(t *testing.T)           { test.Config(t) }
func TestMemBackendGetReader(t *testing.T)        { test.GetReader(t) }
func TestMemBackendLoad(t *testing.T)             { test.Load(t) }
func TestMemBackendWrite(t *testing.T)            { test.Write(t) }
func TestMemBackendGeneric(t *testing.T)          { test.Generic(t) }
func TestMemBackendDelete(t *testing.T)           { test.Delete(t) }
func TestMemBackendCleanup(t *testing.T)          { test.Cleanup(t) }
