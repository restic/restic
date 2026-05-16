package archiver

import (
	"sync"
	"testing"
)

func TestVirtualIdFirstSeenOrder(t *testing.T) {
	m := newMutableDeviceIdMapper()

	// Real 0 is pre-mapped to virtual 0
	if vid, ok := m.GetVirtualId(0); !ok || vid != 0 {
		t.Fatalf("real 0: got (%d, %v), want (0, true)", vid, ok)
	}

	// First-seen order: 100 -> 1, 200 -> 2, 100 again -> 1, 300 -> 3
	cases := []struct {
		realID uint64
		want   uint64
	}{
		{100, 1},
		{200, 2},
		{100, 1},
		{300, 3},
	}
	for _, c := range cases {
		vid, ok := m.GetVirtualId(c.realID)
		if !ok || vid != c.want {
			t.Errorf("real %d: got (%d, %v), want (%d, true)", c.realID, vid, ok, c.want)
		}
	}
}

func TestReadOnlyMapperUnseenReturnsFalse(t *testing.T) {
	m := newMutableDeviceIdMapper()
	m.GetVirtualId(42)
	ro := m.ReadOnlyMapper()

	if vid, ok := ro.GetVirtualId(999); ok || vid != 0 {
		t.Errorf("unseen 999: got (%d, %v), want (0, false)", vid, ok)
	}
}

func TestReadOnlyMapperSeenReturnsId(t *testing.T) {
	m := newMutableDeviceIdMapper()
	vidOwner, _ := m.GetVirtualId(100)
	ro := m.ReadOnlyMapper()

	vidRo, ok := ro.GetVirtualId(100)
	if !ok || vidRo != vidOwner {
		t.Errorf("read-only for seen 100: got (%d, %v), want (%d, true)", vidRo, ok, vidOwner)
	}
}

func TestReadOnlyMapperConcurrent(t *testing.T) {
	m := newMutableDeviceIdMapper()
	ro := m.ReadOnlyMapper()

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = ro.GetVirtualId(1)
			_, _ = ro.GetVirtualId(999)
		}()
	}

	m.GetVirtualId(1)
	wg.Wait()
}

func TestReadOnlyMapperSeesUpdatesFromOwner(t *testing.T) {
	m := newMutableDeviceIdMapper()
	ro := m.ReadOnlyMapper()

	if _, ok := ro.GetVirtualId(1); ok {
		t.Error("read-only saw 1 before owner")
	}

	m.GetVirtualId(1)
	vid, ok := ro.GetVirtualId(1)
	if !ok || vid != 1 {
		t.Errorf("after owner added 1: got (%d, %v), want (1, true)", vid, ok)
	}
}
