package restorer

import (
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/walker"

	"context"
	"os"
	"path/filepath"
)

func DeleteFiles(ctx context.Context, target string, host string, paths []string, tags []restic.TagList, repo restic.Repository, id restic.ID) error {

	var restorefiles, targetfiles, deletefiles []string

	if err := repo.LoadIndex(ctx); err != nil {
		return err
	}

	sn, err := restic.LoadSnapshot(context.TODO(), repo, id)
	if err != nil {
		return err
	}

	err = walker.Walk(ctx, repo, *sn.Tree, nil, func(_ restic.ID, nodepath string, node *restic.Node, err error) (bool, error) {
		if err != nil {
			return false, err
		}
		if node == nil {
			return false, nil
		}
		restorefiles = append(restorefiles, filepath.Join(target, nodepath))
		return false, nil
	})
	if err != nil {
		return err
	}

	err = filepath.Walk(target, func(path string, info os.FileInfo, err error) error {
		targetfiles = append(targetfiles, path)
		return nil
	})
	if err != nil {
		return err
	}

	for _, targetfile := range targetfiles {
		var exists = false
		for _, restorefile := range restorefiles {
			if targetfile == restorefile || targetfile == target {
				exists = true
				break
			}
		}
		if exists != true {
			deletefiles = append(deletefiles, targetfile)
		}
	}

	for _, deletefile := range deletefiles {
		os.RemoveAll(deletefile)
	}

	return nil
}
