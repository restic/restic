package backend_test

import (
	"testing"

	"github.com/restic/restic/backend"
	. "github.com/restic/restic/test"
)

func str2id(s string) backend.ID {
	id, err := backend.ParseID(s)
	if err != nil {
		panic(err)
	}

	return id
}

type mockBackend struct {
	list func(backend.Type, <-chan struct{}) <-chan string
}

func (m mockBackend) List(t backend.Type, done <-chan struct{}) <-chan string {
	return m.list(t, done)
}

var samples = backend.IDs{
	str2id("20bdc1402a6fc9b633aaffffffffffffffffffffffffffffffffffffffffffff"),
	str2id("20bdc1402a6fc9b633ccd578c4a92d0f4ef1a457fa2e16c596bc73fb409d6cc0"),
	str2id("20bdc1402a6fc9b633ffffffffffffffffffffffffffffffffffffffffffffff"),
	str2id("20ff988befa5fc40350f00d531a767606efefe242c837aaccb80673f286be53d"),
	str2id("326cb59dfe802304f96ee9b5b9af93bdee73a30f53981e5ec579aedb6f1d0f07"),
	str2id("86b60b9594d1d429c4aa98fa9562082cabf53b98c7dc083abe5dae31074dd15a"),
	str2id("96c8dbe225079e624b5ce509f5bd817d1453cd0a85d30d536d01b64a8669aeae"),
	str2id("fa31d65b87affcd167b119e9d3d2a27b8236ca4836cb077ed3e96fcbe209b792"),
}

func TestPrefixLength(t *testing.T) {
	list := samples

	m := mockBackend{}
	m.list = func(t backend.Type, done <-chan struct{}) <-chan string {
		ch := make(chan string)
		go func() {
			defer close(ch)
			for _, id := range list {
				select {
				case ch <- id.String():
				case <-done:
					return
				}
			}
		}()
		return ch
	}

	l, err := backend.PrefixLength(m, backend.Snapshot)
	OK(t, err)
	Equals(t, 19, l)

	list = samples[:3]
	l, err = backend.PrefixLength(m, backend.Snapshot)
	OK(t, err)
	Equals(t, 19, l)

	list = samples[3:]
	l, err = backend.PrefixLength(m, backend.Snapshot)
	OK(t, err)
	Equals(t, 8, l)
}
