package backend

import (
	"errors"
	"sort"
	"sync"
)

type IDSet struct {
	list IDs
	m    sync.Mutex
}

func NewIDSet() *IDSet {
	return &IDSet{
		list: make(IDs, 0),
	}
}

func (s *IDSet) find(id ID) (int, error) {
	pos := sort.Search(len(s.list), func(i int) bool {
		return id.Compare(s.list[i]) >= 0
	})

	if pos < len(s.list) {
		candID := s.list[pos]
		if id.Compare(candID) == 0 {
			return pos, nil
		}
	}

	return pos, errors.New("ID not found")
}

func (s *IDSet) insert(id ID) {
	pos, err := s.find(id)
	if err == nil {
		// already present
		return
	}

	// insert blob
	// https://code.google.com/p/go-wiki/wiki/SliceTricks
	s.list = append(s.list, ID{})
	copy(s.list[pos+1:], s.list[pos:])
	s.list[pos] = id

	return
}

func (s *IDSet) Insert(id ID) {
	s.m.Lock()
	defer s.m.Unlock()

	s.insert(id)
}

func (s *IDSet) Find(id ID) error {
	s.m.Lock()
	defer s.m.Unlock()

	_, err := s.find(id)
	if err != nil {
		return err
	}

	return nil
}
