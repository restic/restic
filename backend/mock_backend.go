package backend

import (
	"errors"
	"io"
)

// MockBackend implements a backend whose functions can be specified. This
// should only be used for tests.
type MockBackend struct {
	CloseFn     func() error
	CreateFn    func() (Blob, error)
	LoadFn      func(h Handle, p []byte, off int64) (int, error)
	StatFn      func(h Handle) (BlobInfo, error)
	GetReaderFn func(Type, string, uint, uint) (io.ReadCloser, error)
	ListFn      func(Type, <-chan struct{}) <-chan string
	RemoveFn    func(Type, string) error
	TestFn      func(Type, string) (bool, error)
	DeleteFn    func() error
	LocationFn  func() string
}

func (m *MockBackend) Close() error {
	if m.CloseFn == nil {
		return nil
	}

	return m.CloseFn()
}

func (m *MockBackend) Location() string {
	if m.LocationFn == nil {
		return ""
	}

	return m.LocationFn()
}

func (m *MockBackend) Create() (Blob, error) {
	if m.CreateFn == nil {
		return nil, errors.New("not implemented")
	}

	return m.CreateFn()
}

func (m *MockBackend) Load(h Handle, p []byte, off int64) (int, error) {
	if m.LoadFn == nil {
		return 0, errors.New("not implemented")
	}

	return m.LoadFn(h, p, off)
}

func (m *MockBackend) Stat(h Handle) (BlobInfo, error) {
	if m.StatFn == nil {
		return BlobInfo{}, errors.New("not implemented")
	}

	return m.StatFn(h)
}

func (m *MockBackend) GetReader(t Type, name string, offset, len uint) (io.ReadCloser, error) {
	if m.GetReaderFn == nil {
		return nil, errors.New("not implemented")
	}

	return m.GetReaderFn(t, name, offset, len)
}

func (m *MockBackend) List(t Type, done <-chan struct{}) <-chan string {
	if m.ListFn == nil {
		ch := make(chan string)
		close(ch)
		return ch
	}

	return m.ListFn(t, done)
}

func (m *MockBackend) Remove(t Type, name string) error {
	if m.RemoveFn == nil {
		return errors.New("not implemented")
	}

	return m.RemoveFn(t, name)
}

func (m *MockBackend) Test(t Type, name string) (bool, error) {
	if m.TestFn == nil {
		return false, errors.New("not implemented")
	}

	return m.TestFn(t, name)
}

func (m *MockBackend) Delete() error {
	if m.DeleteFn == nil {
		return errors.New("not implemented")
	}

	return m.DeleteFn()
}

// Make sure that MockBackend implements the backend interface.
var _ Backend = &MockBackend{}
