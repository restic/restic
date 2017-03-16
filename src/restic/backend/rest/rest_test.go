package rest_test

import (
	"fmt"
	"net/url"
	"os"
	"restic"

	"restic/backend/rest"
	"restic/backend/test"
	. "restic/test"
)

//go:generate go run ../test/generate_backend_tests.go

func init() {
	if TestRESTServer == "" {
		SkipMessage = "REST test server not available"
		return
	}

	url, err := url.Parse(TestRESTServer)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid url: %v\n", err)
		return
	}

	cfg := rest.Config{
		URL: url,
	}

	test.CreateFn = func() (restic.Backend, error) {
		return rest.Create(cfg)
	}

	test.OpenFn = func() (restic.Backend, error) {
		return rest.Open(cfg)
	}
}
