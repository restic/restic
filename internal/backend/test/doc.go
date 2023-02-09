// Package test contains a test suite with benchmarks for restic backends.
//
// # Overview
//
// For the test suite to work a few functions need to be implemented to create
// new config, create a backend, open it and run cleanup tasks afterwards. The
// Suite struct has fields for each function.
//
// So for a new backend, a Suite needs to be built with callback functions,
// then the methods RunTests() and RunBenchmarks() can be used to run the
// individual tests and benchmarks as subtests/subbenchmarks.
//
// # Example
//
// Assuming a *Suite is returned by newTestSuite(), the tests and benchmarks
// can be run like this:
//
//	func newTestSuite(t testing.TB) *test.Suite {
//		return &test.Suite{
//			Create: func(cfg interface{}) (restic.Backend, error) {
//				[...]
//			},
//			[...]
//		}
//	}
//
//	func TestSuiteBackendMem(t *testing.T) {
//		newTestSuite(t).RunTests(t)
//	}
//
//	func BenchmarkSuiteBackendMem(b *testing.B) {
//		newTestSuite(b).RunBenchmarks(b)
//	}
//
// The functions are run in alphabetical order.
//
// # Add new tests
//
// A new test or benchmark can be added by implementing a method on *Suite
// with the name starting with "Test" and a single *testing.T parameter for
// test. For benchmarks, the name must start with "Benchmark" and the parameter
// is a *testing.B
package test
