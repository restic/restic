package inproc

import (
	"fmt"
	"net/http"
	"net/url"

	"github.com/restic/restic/internal/backend/rest"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/limiter"
)

// Backend is used to access data stored somewhere via rclone.
type Backend struct {
	*rest.Backend
	tr http.RoundTripper
}

// New initializes a Backend and starts the process.
func New(cfg Config, lim limiter.Limiter) (*Backend, error) {
	service, err := cfg.provider.NewService(cfg.Remote)
	if err != nil {
		return nil, err
	}

	be := &Backend{
		tr: service,
	}
	return be, nil
}

// Open starts an rclone process with the given config.
func Open(cfg Config, lim limiter.Limiter) (*Backend, error) {
	be, err := New(cfg, lim)
	if err != nil {
		return nil, err
	}

	url, err := url.Parse("http://localhost/")
	if err != nil {
		return nil, err
	}

	restConfig := rest.Config{
		Connections: cfg.Connections,
		URL:         url,
	}

	restBackend, err := rest.Open(restConfig, debug.RoundTripper(be.tr))
	if err != nil {
		return nil, err
	}

	be.Backend = restBackend
	return be, nil
}

type ServiceProvider interface {
	NewService(args string) (http.RoundTripper, error)
}

var providers = make(map[string]ServiceProvider)

// RegisterServiceProvider register a provider that can create an http.RoundTripper
func RegisterServiceProvider(name string, provider ServiceProvider) {
	providers[name] = provider
}

func findServiceProvider(name string) (ServiceProvider, error) {
	if p, ok := providers[name]; ok {
		return p, nil
	}
	return nil, fmt.Errorf("service %s not found", name)
}
