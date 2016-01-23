// DO NOT EDIT, AUTOMATICALLY GENERATED
package test_test

import (
	"testing"

	"github.com/restic/restic/backend/test"
)

func TestTestBackendCreate(t *testing.T)    { test.Create(t) }
func TestTestBackendOpen(t *testing.T)      { test.Open(t) }
func TestTestBackendLocation(t *testing.T)  { test.Location(t) }
func TestTestBackendConfig(t *testing.T)    { test.Config(t) }
func TestTestBackendGetReader(t *testing.T) { test.GetReader(t) }
func TestTestBackendLoad(t *testing.T)      { test.Load(t) }
func TestTestBackendWrite(t *testing.T)     { test.Write(t) }
func TestTestBackendGeneric(t *testing.T)   { test.Generic(t) }
func TestTestBackendDelete(t *testing.T)    { test.Delete(t) }
func TestTestBackendCleanup(t *testing.T)   { test.Cleanup(t) }
