package restic

import (
	"testing"
)

type mockBackend struct {
	list func(FileType, <-chan struct{}) <-chan string
}

func (m mockBackend) List(t FileType, done <-chan struct{}) <-chan string {
	return m.list(t, done)
}

var samples = IDs{
	TestParseID("20bdc1402a6fc9b633aaffffffffffffffffffffffffffffffffffffffffffff"),
	TestParseID("20bdc1402a6fc9b633ccd578c4a92d0f4ef1a457fa2e16c596bc73fb409d6cc0"),
	TestParseID("20bdc1402a6fc9b633ffffffffffffffffffffffffffffffffffffffffffffff"),
	TestParseID("20ff988befa5fc40350f00d531a767606efefe242c837aaccb80673f286be53d"),
	TestParseID("326cb59dfe802304f96ee9b5b9af93bdee73a30f53981e5ec579aedb6f1d0f07"),
	TestParseID("86b60b9594d1d429c4aa98fa9562082cabf53b98c7dc083abe5dae31074dd15a"),
	TestParseID("96c8dbe225079e624b5ce509f5bd817d1453cd0a85d30d536d01b64a8669aeae"),
	TestParseID("fa31d65b87affcd167b119e9d3d2a27b8236ca4836cb077ed3e96fcbe209b792"),
}

func TestPrefixLength(t *testing.T) {
	list := samples

	m := mockBackend{}
	m.list = func(t FileType, done <-chan struct{}) <-chan string {
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

	l, err := PrefixLength(m, SnapshotFile)
	if err != nil {
		t.Error(err)
	}
	if l != 19 {
		t.Errorf("wrong prefix length returned, want %d, got %d", 19, l)
	}

	list = samples[:3]
	l, err = PrefixLength(m, SnapshotFile)
	if err != nil {
		t.Error(err)
	}
	if l != 19 {
		t.Errorf("wrong prefix length returned, want %d, got %d", 19, l)
	}

	list = samples[3:]
	l, err = PrefixLength(m, SnapshotFile)
	if err != nil {
		t.Error(err)
	}
	if l != 8 {
		t.Errorf("wrong prefix length returned, want %d, got %d", 8, l)
	}
}
