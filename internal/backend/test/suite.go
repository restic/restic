package test

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/backend/location"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/test"
)

// Suite implements a test suite for restic backends.
type Suite[C any] struct {
	// Config should be used to configure the backend.
	Config *C

	// NewConfig returns a config for a new temporary backend that will be used in tests.
	NewConfig func() (*C, error)

	// Factory contains a factory that can be used to create or open a repository for the tests.
	Factory location.Factory

	// MinimalData instructs the tests to not use excessive data.
	MinimalData bool

	// WaitForDelayedRemoval is set to a non-zero value to instruct the test
	// suite to wait for this amount of time until a file that was removed
	// really disappeared.
	WaitForDelayedRemoval time.Duration

	// ErrorHandler allows ignoring certain errors.
	ErrorHandler func(testing.TB, backend.Backend, error) error
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

	s.cleanup(t)
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

	s.cleanup(b)
}

func (s *Suite[C]) createOrError() (backend.Backend, error) {
	tr, err := backend.Transport(backend.TransportOptions{})
	if err != nil {
		return nil, fmt.Errorf("cannot create transport for tests: %v", err)
	}

	be, err := s.Factory.Create(context.TODO(), s.Config, tr, nil)
	if err != nil {
		return nil, err
	}

	_, err = be.Stat(context.TODO(), backend.Handle{Type: backend.ConfigFile})
	if err != nil && !be.IsNotExist(err) {
		return nil, err
	}

	if err == nil {
		return nil, errors.New("config already exists")
	}

	return be, nil
}

func (s *Suite[C]) create(t testing.TB) backend.Backend {
	be, err := s.createOrError()
	if err != nil {
		t.Fatal(err)
	}
	return be
}

func (s *Suite[C]) open(t testing.TB) backend.Backend {
	tr, err := backend.Transport(backend.TransportOptions{})
	if err != nil {
		t.Fatalf("cannot create transport for tests: %v", err)
	}

	be, err := s.Factory.Open(context.TODO(), s.Config, tr, nil)
	if err != nil {
		t.Fatal(err)
	}
	return be
}

func (s *Suite[C]) cleanup(t testing.TB) {
	be := s.open(t)
	if err := be.Delete(context.TODO()); err != nil {
		t.Fatal(err)
	}
	s.close(t, be)
}

func (s *Suite[C]) close(t testing.TB, be backend.Backend) {
	err := be.Close()
	if err != nil {
		t.Fatal(err)
	}
}
