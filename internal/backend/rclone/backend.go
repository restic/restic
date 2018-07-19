package rclone

import (
	"time"

	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/limiter"
	"github.com/restic/restic/internal/backend/reststdiohttp2"
)

type Backend struct {
	*reststdiohttp2.Backend
}

// New initializes a Backend and starts the process.
func New(cfg Config, lim limiter.Limiter) (*Backend, error) {
	var (
		args []string
	)

	// build program args, start with the program
	if cfg.Program != "" {
		a, err := backend.SplitShellStrings(cfg.Program)
		if err != nil {
			return nil, err
		}
		args = append(args, a...)
	} else {
		args = append(args, "rclone")
	}

	// then add the arguments
	if cfg.Args != "" {
		a, err := backend.SplitShellStrings(cfg.Args)
		if err != nil {
			return nil, err
		}

		args = append(args, a...)
	} else {
		args = append(args,
			"serve", "restic", "--stdio",
			"--b2-hard-delete", "--drive-use-trash=false")
	}

	// finally, add the remote
	args = append(args, cfg.Remote)

	debug.Log("running command: %v %v", args)

	be, err := reststdiohttp2.New(args, lim, warmupTime, waitForExit, cfg.Connections)
	if err != nil {
		return nil, err
	}

	return &Backend{
		be,
	}, nil

}

// Open starts an rclone process with the given config.
func Open(cfg Config, lim limiter.Limiter) (*Backend, error) {
	be, err := New(cfg, lim)
	if err != nil {
		return nil, err
	}

	err = be.Open()
	if err != nil {
		return nil, err
	}

	return be, nil
}

// Create initializes a new restic repo with rclone.
func Create(cfg Config, lim limiter.Limiter) (*Backend, error) {
	be, err := New(cfg, lim)
	if err != nil {
		return nil, err
	}

	err = be.Create()
	if err != nil {
		return nil, err
	}

	return be, nil
}

const waitForExit = 5 * time.Second
const warmupTime = 60 * time.Second

// Close terminates the backend.
func (be *Backend) Close() error {
	debug.Log("exiting rclone")
	return be.Backend.Close()
}
