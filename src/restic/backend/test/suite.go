package test

import (
	"restic"
	"restic/test"
	"testing"
)

// Suite implements a test suite for restic backends.
type Suite struct {
	Config interface{}

	// NewConfig returns a config for a new temporary backend that will be used in tests.
	NewConfig func() (interface{}, error)

	// CreateFn is a function that creates a temporary repository for the tests.
	Create func(cfg interface{}) (restic.Backend, error)

	// OpenFn is a function that opens a previously created temporary repository.
	Open func(cfg interface{}) (restic.Backend, error)

	// CleanupFn removes data created during the tests.
	Cleanup func(cfg interface{}) error

	// MinimalData instructs the tests to not use excessive data.
	MinimalData bool
}

// RunTests executes all defined tests as subtests of t.
func (s *Suite) RunTests(t *testing.T) {
	var err error
	s.Config, err = s.NewConfig()
	if err != nil {
		t.Fatal(err)
	}

	// test create/open functions first
	be := s.create(t)
	s.close(t, be)

	for _, test := range testFunctions {
		t.Run(test.Name, func(t *testing.T) {
			test.Fn(t, s)
		})
	}

	if !test.TestCleanupTempDirs {
		t.Logf("not cleaning up backend")
		return
	}

	if err = s.Cleanup(s.Config); err != nil {
		t.Fatal(err)
	}
}

// RunBenchmarks executes all defined benchmarks as subtests of b.
func (s *Suite) RunBenchmarks(b *testing.B) {
	var err error
	s.Config, err = s.NewConfig()
	if err != nil {
		b.Fatal(err)
	}

	// test create/open functions first
	be := s.create(b)
	s.close(b, be)

	for _, test := range benchmarkFunctions {
		b.Run(test.Name, func(b *testing.B) {
			test.Fn(b, s)
		})
	}

	if !test.TestCleanupTempDirs {
		b.Logf("not cleaning up backend")
		return
	}

	if err = s.Cleanup(s.Config); err != nil {
		b.Fatal(err)
	}
}

func (s *Suite) create(t testing.TB) restic.Backend {
	be, err := s.Create(s.Config)
	if err != nil {
		t.Fatal(err)
	}
	return be
}

func (s *Suite) open(t testing.TB) restic.Backend {
	be, err := s.Open(s.Config)
	if err != nil {
		t.Fatal(err)
	}
	return be
}

func (s *Suite) close(t testing.TB, be restic.Backend) {
	err := be.Close()
	if err != nil {
		t.Fatal(err)
	}
}
