package main

import "github.com/fd0/khepri/backend"

// func fsck_tree(be backend.Server, id backend.ID) (bool, error) {
// 	log.Printf("  checking dir %s", id)

// 	buf, err := be.GetBlob(id)
// 	if err != nil {
// 		return false, err
// 	}

// 	tree := &khepri.Tree{}
// 	err = json.Unmarshal(buf, tree)
// 	if err != nil {
// 		return false, err
// 	}

// 	if !id.Equal(backend.IDFromData(buf)) {
// 		return false, nil
// 	}

// 	return true, nil
// }

// func fsck_snapshot(be backend.Server, id backend.ID) (bool, error) {
// 	log.Printf("checking snapshot %s", id)

// 	sn, err := khepri.LoadSnapshot(be, id)
// 	if err != nil {
// 		return false, err
// 	}

// 	return fsck_tree(be, sn.Content)
// }

func commandFsck(be backend.Server, args []string) error {
	// var snapshots backend.IDs
	// var err error

	// if len(args) != 0 {
	// 	snapshots = make(backend.IDs, 0, len(args))

	// 	for _, arg := range args {
	// 		id, err := backend.ParseID(arg)
	// 		if err != nil {
	// 			log.Fatal(err)
	// 		}

	// 		snapshots = append(snapshots, id)
	// 	}
	// } else {
	// 	snapshots, err = be.ListRefs()

	// 	if err != nil {
	// 		log.Fatalf("error reading list of snapshot IDs: %v", err)
	// 	}
	// }

	// log.Printf("checking %d snapshots", len(snapshots))

	// for _, id := range snapshots {
	// 	ok, err := fsck_snapshot(be, id)

	// 	if err != nil {
	// 		log.Printf("error checking snapshot %s: %v", id, err)
	// 		continue
	// 	}

	// 	if !ok {
	// 		log.Printf("snapshot %s failed", id)
	// 	}
	// }

	return nil
}
