// DO NOT EDIT, AUTOMATICALLY GENERATED
package local_test

import (
	"testing"

	"github.com/restic/restic/backend/test"
)

func TestLocalBackendCreate(t *testing.T)    { test.Create(t) }
func TestLocalBackendOpen(t *testing.T)      { test.Open(t) }
func TestLocalBackendLocation(t *testing.T)  { test.Location(t) }
func TestLocalBackendConfig(t *testing.T)    { test.Config(t) }
func TestLocalBackendGetReader(t *testing.T) { test.GetReader(t) }
func TestLocalBackendLoad(t *testing.T)      { test.Load(t) }
func TestLocalBackendWrite(t *testing.T)     { test.Write(t) }
func TestLocalBackendGeneric(t *testing.T)   { test.Generic(t) }
func TestLocalBackendDelete(t *testing.T)    { test.Delete(t) }
func TestLocalBackendCleanup(t *testing.T)   { test.Cleanup(t) }
