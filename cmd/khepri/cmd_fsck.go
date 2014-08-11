package main

import (
	"crypto/sha256"
	"encoding/json"
	"log"

	"github.com/fd0/khepri"
)

func fsck_tree(repo *khepri.Repository, id khepri.ID) (bool, error) {
	log.Printf("  checking dir %s", id)

	rd, err := repo.Get(khepri.TYPE_BLOB, id)
	if err != nil {
		return false, err
	}

	hr := khepri.NewHashingReader(rd, sha256.New)
	dec := json.NewDecoder(hr)

	tree := &khepri.Tree{}
	err = dec.Decode(tree)

	if err != nil {
		return false, err
	}

	if !id.Equal(hr.Hash()) {
		return false, nil
	}

	return true, nil
}

func fsck_snapshot(repo *khepri.Repository, id khepri.ID) (bool, error) {
	log.Printf("checking snapshot %s", id)

	sn, err := khepri.LoadSnapshot(repo, id)
	if err != nil {
		return false, err
	}

	return fsck_tree(repo, sn.Content)
}

func commandFsck(repo *khepri.Repository, args []string) error {
	var snapshots khepri.IDs
	var err error

	if len(args) != 0 {
		snapshots = make(khepri.IDs, 0, len(args))

		for _, arg := range args {
			id, err := khepri.ParseID(arg)
			if err != nil {
				log.Fatal(err)
			}

			snapshots = append(snapshots, id)
		}
	} else {
		snapshots, err = repo.List(khepri.TYPE_REF)

		if err != nil {
			log.Fatalf("error reading list of snapshot IDs: %v", err)
		}
	}

	log.Printf("checking %d snapshots", len(snapshots))

	for _, id := range snapshots {
		ok, err := fsck_snapshot(repo, id)

		if err != nil {
			log.Printf("error checking snapshot %s: %v", id, err)
			continue
		}

		if !ok {
			log.Printf("snapshot %s failed", id)
		}
	}

	return nil
}
