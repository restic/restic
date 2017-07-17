package profile_test

import "github.com/pkg/profile"

func ExampleTraceProfile() {
	// use execution tracing, rather than the default cpu profiling.
	defer profile.Start(profile.TraceProfile).Stop()
}
