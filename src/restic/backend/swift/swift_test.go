package swift_test

import (
	"fmt"
	"math/rand"
	"restic"
	"time"

	"restic/errors"

	"restic/backend/swift"
	"restic/backend/test"
	. "restic/test"

	swiftclient "github.com/ncw/swift"
)

//go:generate go run ../test/generate_backend_tests.go

func init() {
	if TestSwiftServer == "" {
		SkipMessage = "swift test server not available"
		return
	}

	// Generate random container name to allow simultaneous test
	// on the same swift backend
	containerName := fmt.Sprintf(
		"restictestcontainer_%d_%d",
		time.Now().Unix(),
		rand.Uint32(),
	)

	cfg := swift.Config{
		Container:  containerName,
		StorageURL: TestSwiftServer,
		AuthToken:  TestSwiftToken,
	}

	test.CreateFn = func() (restic.Backend, error) {
		be, err := swift.Open(cfg)
		if err != nil {
			return nil, err
		}

		exists, err := be.Test(restic.Handle{Type: restic.ConfigFile})
		if err != nil {
			return nil, err
		}

		if exists {
			return nil, errors.New("config already exists")
		}

		return be, nil
	}

	test.OpenFn = func() (restic.Backend, error) {
		return swift.Open(cfg)
	}

	test.CleanupFn = func() error {
		client := swiftclient.Connection{
			StorageUrl: TestSwiftServer,
			AuthToken:  TestSwiftToken,
		}
		objects, err := client.ObjectsAll(containerName, nil)
		if err != nil {
			return err
		}
		for _, o := range objects {
			client.ObjectDelete(containerName, o.Name)
		}
		return client.ContainerDelete(containerName)
	}
}
