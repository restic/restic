package main

import (
	"github.com/restic/restic/internal/restic"
)

func getIDsFromFiles(files []string) (restic.IDSet, error) {
	ids := restic.NewIDSet()

	for _, file := range files {
		fromfile, err := readLines(file)
		if err != nil {
			return nil, err
		}

		// read IDs from file
		for _, line := range fromfile {
			id, err := restic.ParseID(line)
			if err != nil {
				return nil, err
			}
			ids.Insert(id)
		}
	}
	return ids, nil
}
