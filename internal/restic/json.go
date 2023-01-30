package restic

import (
	"context"
	"encoding/json"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
)

// LoadJSONUnpacked decrypts the data and afterwards calls json.Unmarshal on
// the item.
func LoadJSONUnpacked(ctx context.Context, repo LoaderUnpacked, t FileType, id ID, item interface{}) (err error) {
	buf, err := repo.LoadUnpacked(ctx, t, id)
	if err != nil {
		return err
	}

	return json.Unmarshal(buf, item)
}

// SaveJSONUnpacked serialises item as JSON and encrypts and saves it in the
// backend as type t, without a pack. It returns the storage hash.
func SaveJSONUnpacked(ctx context.Context, repo SaverUnpacked, t FileType, item interface{}) (ID, error) {
	debug.Log("save new blob %v", t)
	plaintext, err := json.Marshal(item)
	if err != nil {
		return ID{}, errors.Wrap(err, "json.Marshal")
	}

	return repo.SaveUnpacked(ctx, t, plaintext)
}
