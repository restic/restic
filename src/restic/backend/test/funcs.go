// DO NOT EDIT, AUTOMATICALLY GENERATED

package test

import (
	"testing"
)

var testFunctions = []struct {
	Name string
	Fn   func(testing.TB, *Suite)
}{
	{"CreateWithConfig", BackendTestCreateWithConfig},
	{"Location", BackendTestLocation},
	{"Config", BackendTestConfig},
	{"Load", BackendTestLoad},
	{"Save", BackendTestSave},
	{"SaveFilenames", BackendTestSaveFilenames},
	{"Backend", BackendTestBackend},
	{"Delete", BackendTestDelete},
}

var benchmarkFunctions = []struct {
	Name string
	Fn   func(*testing.B, *Suite)
}{
	{"LoadFile", BackendBenchmarkLoadFile},
	{"LoadPartialFile", BackendBenchmarkLoadPartialFile},
	{"LoadPartialFileOffset", BackendBenchmarkLoadPartialFileOffset},
}
