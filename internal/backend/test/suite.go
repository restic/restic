package test

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/test"
)

// Suite implements a test suite for restic backends.
type Suite[C any] struct {
	// Config should be used to configure the backend.
	Config C

	// NewConfig returns a config for a new temporary backend that will be used in tests.
	NewConfig func() (C, error)

	// CreateFn is a function that creates a temporary repository for the tests.
	Create func(cfg C) (restic.Backend, error)

	// OpenFn is a function that opens a previously created temporary repository.
	Open func(cfg C) (restic.Backend, error)

	// CleanupFn removes data created during the tests.
	Cleanup func(cfg C) error

	// MinimalData instructs the tests to not use excessive data.
	MinimalData bool

	// WaitForDelayedRemoval is set to a non-zero value to instruct the test
	// suite to wait for this amount of time until a file that was removed
	// really disappeared.
	WaitForDelayedRemoval time.Duration

	// ErrorHandler allows ignoring certain errors.
	ErrorHandler func(testing.TB, restic.Backend, error) error
}

// RunTests executes all defined tests as subtests of t.
func (s *Suite[C]) RunTests(t *testing.T) {
	var err error
	s.Config, err = s.NewConfig()
	if err != nil {
		t.Fatal(err)
	}

	// test create/open functions first
	be := s.create(t)
	s.close(t, be)

	for _, test := range s.testFuncs(t) {
		t.Run(test.Name, test.Fn)
	}

	if !test.TestCleanupTempDirs {
		t.Logf("not cleaning up backend")
		return
	}

	if s.Cleanup != nil {
		if err = s.Cleanup(s.Config); err != nil {
			t.Fatal(err)
		}
	}
}

type testFunction struct {
	Name string
	Fn   func(*testing.T)
}

func (s *Suite[C]) testFuncs(t testing.TB) (funcs []testFunction) {
	tpe := reflect.TypeOf(s)
	v := reflect.ValueOf(s)

	for i := 0; i < tpe.NumMethod(); i++ {
		methodType := tpe.Method(i)
		name := methodType.Name

		// discard functions which do not have the right name
		if !strings.HasPrefix(name, "Test") {
			continue
		}

		iface := v.Method(i).Interface()
		f, ok := iface.(func(*testing.T))
		if !ok {
			t.Logf("warning: function %v of *Suite has the wrong signature for a test function\nwant: func(*testing.T),\nhave: %T",
				name, iface)
			continue
		}

		funcs = append(funcs, testFunction{
			Name: name,
			Fn:   f,
		})
	}

	return funcs
}

type benchmarkFunction struct {
	Name string
	Fn   func(*testing.B)
}

func (s *Suite[C]) benchmarkFuncs(t testing.TB) (funcs []benchmarkFunction) {
	tpe := reflect.TypeOf(s)
	v := reflect.ValueOf(s)

	for i := 0; i < tpe.NumMethod(); i++ {
		methodType := tpe.Method(i)
		name := methodType.Name

		// discard functions which do not have the right name
		if !strings.HasPrefix(name, "Benchmark") {
			continue
		}

		iface := v.Method(i).Interface()
		f, ok := iface.(func(*testing.B))
		if !ok {
			t.Logf("warning: function %v of *Suite has the wrong signature for a test function\nwant: func(*testing.T),\nhave: %T",
				name, iface)
			continue
		}

		funcs = append(funcs, benchmarkFunction{
			Name: name,
			Fn:   f,
		})
	}

	return funcs
}

// RunBenchmarks executes all defined benchmarks as subtests of b.
func (s *Suite[C]) RunBenchmarks(b *testing.B) {
	var err error
	s.Config, err = s.NewConfig()
	if err != nil {
		b.Fatal(err)
	}

	// test create/open functions first
	be := s.create(b)
	s.close(b, be)

	for _, test := range s.benchmarkFuncs(b) {
		b.Run(test.Name, test.Fn)
	}

	if !test.TestCleanupTempDirs {
		b.Logf("not cleaning up backend")
		return
	}

	if err = s.Cleanup(s.Config); err != nil {
		b.Fatal(err)
	}
}

func (s *Suite[C]) create(t testing.TB) restic.Backend {
	be, err := s.Create(s.Config)
	if err != nil {
		t.Fatal(err)
	}
	return be
}

func (s *Suite[C]) open(t testing.TB) restic.Backend {
	be, err := s.Open(s.Config)
	if err != nil {
		t.Fatal(err)
	}
	return be
}

func (s *Suite[C]) close(t testing.TB, be restic.Backend) {
	err := be.Close()
	if err != nil {
		t.Fatal(err)
	}
}
