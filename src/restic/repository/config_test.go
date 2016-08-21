package repository_test

import (
	"testing"

	"restic/backend"
	"restic/repository"
	. "restic/test"
)

type saver func(backend.Type, interface{}) (backend.ID, error)

func (s saver) SaveJSONUnpacked(t backend.Type, arg interface{}) (backend.ID, error) {
	return s(t, arg)
}

type loader func(backend.Type, backend.ID, interface{}) error

func (l loader) LoadJSONUnpacked(t backend.Type, id backend.ID, arg interface{}) error {
	return l(t, id, arg)
}

func TestConfig(t *testing.T) {
	resultConfig := repository.Config{}
	save := func(tpe backend.Type, arg interface{}) (backend.ID, error) {
		Assert(t, tpe == backend.Config,
			"wrong backend type: got %v, wanted %v",
			tpe, backend.Config)

		cfg := arg.(repository.Config)
		resultConfig = cfg
		return backend.ID{}, nil
	}

	cfg1, err := repository.CreateConfig()
	OK(t, err)

	_, err = saver(save).SaveJSONUnpacked(backend.Config, cfg1)

	load := func(tpe backend.Type, id backend.ID, arg interface{}) error {
		Assert(t, tpe == backend.Config,
			"wrong backend type: got %v, wanted %v",
			tpe, backend.Config)

		cfg := arg.(*repository.Config)
		*cfg = resultConfig
		return nil
	}

	cfg2, err := repository.LoadConfig(loader(load))
	OK(t, err)

	Assert(t, cfg1 == cfg2,
		"configs aren't equal: %v != %v", cfg1, cfg2)
}
