package repository_test

import (
	"restic"
	"testing"

	"restic/repository"
	. "restic/test"
)

type saver func(restic.FileType, interface{}) (restic.ID, error)

func (s saver) SaveJSONUnpacked(t restic.FileType, arg interface{}) (restic.ID, error) {
	return s(t, arg)
}

type loader func(restic.FileType, restic.ID, interface{}) error

func (l loader) LoadJSONUnpacked(t restic.FileType, id restic.ID, arg interface{}) error {
	return l(t, id, arg)
}

func TestConfig(t *testing.T) {
	resultConfig := repository.Config{}
	save := func(tpe restic.FileType, arg interface{}) (restic.ID, error) {
		Assert(t, tpe == restic.ConfigFile,
			"wrong backend type: got %v, wanted %v",
			tpe, restic.ConfigFile)

		cfg := arg.(repository.Config)
		resultConfig = cfg
		return restic.ID{}, nil
	}

	cfg1, err := repository.CreateConfig()
	OK(t, err)

	_, err = saver(save).SaveJSONUnpacked(restic.ConfigFile, cfg1)

	load := func(tpe restic.FileType, id restic.ID, arg interface{}) error {
		Assert(t, tpe == restic.ConfigFile,
			"wrong backend type: got %v, wanted %v",
			tpe, restic.ConfigFile)

		cfg := arg.(*repository.Config)
		*cfg = resultConfig
		return nil
	}

	cfg2, err := repository.LoadConfig(loader(load))
	OK(t, err)

	Assert(t, cfg1 == cfg2,
		"configs aren't equal: %v != %v", cfg1, cfg2)
}
