package restic_test

import (
	"context"
	"testing"

	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
)

type saver func(restic.FileType, interface{}) (restic.ID, error)

func (s saver) SaveJSONUnpacked(t restic.FileType, arg interface{}) (restic.ID, error) {
	return s(t, arg)
}

type loader func(context.Context, restic.FileType, restic.ID, interface{}) error

func (l loader) LoadJSONUnpacked(ctx context.Context, t restic.FileType, id restic.ID, arg interface{}) error {
	return l(ctx, t, id, arg)
}

func TestConfig(t *testing.T) {
	resultConfig := restic.Config{}
	save := func(tpe restic.FileType, arg interface{}) (restic.ID, error) {
		rtest.Assert(t, tpe == restic.ConfigFile,
			"wrong backend type: got %v, wanted %v",
			tpe, restic.ConfigFile)

		cfg := arg.(restic.Config)
		resultConfig = cfg
		return restic.ID{}, nil
	}

	cfg1, err := restic.CreateConfig()
	rtest.OK(t, err)

	_, err = saver(save).SaveJSONUnpacked(restic.ConfigFile, cfg1)
	rtest.OK(t, err)

	load := func(ctx context.Context, tpe restic.FileType, id restic.ID, arg interface{}) error {
		rtest.Assert(t, tpe == restic.ConfigFile,
			"wrong backend type: got %v, wanted %v",
			tpe, restic.ConfigFile)

		cfg := arg.(*restic.Config)
		*cfg = resultConfig
		return nil
	}

	cfg2, err := restic.LoadConfig(context.TODO(), loader(load))
	rtest.OK(t, err)

	rtest.Assert(t, cfg1 == cfg2,
		"configs aren't equal: %v != %v", cfg1, cfg2)
}
