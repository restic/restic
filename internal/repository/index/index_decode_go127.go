//go:build go1.27

package index

import (
	"bytes"
	"encoding/json/jsontext"
	"io"
	"math"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/repository/pack"
	"github.com/restic/restic/internal/restic"
)

// Bits for the members each object in the index format defines. A repeated
// known member is rejected here.
const (
	indexSeenPacks = 1 << iota
)

const (
	packSeenID = 1 << iota
	packSeenBlobs
)

const (
	blobSeenID = 1 << iota
	blobSeenType
	blobSeenOffset
	blobSeenLength
	blobSeenUncompressedLength
)

// decodeIndexJSON directly streams the index JSON into an Index without intermediate objects.
// Hand-rolled decoder for packJSON to significantly improve performance.
func decodeIndexJSON(buf []byte) (*Index, error) {
	// use custom duplicate name check as jsontext's one is much slower.
	dec := jsontext.NewDecoder(bytes.NewBuffer(buf), jsontext.AllowDuplicateNames(true))
	idx := NewIndex()

	if err := assertJSONKind(dec, jsontext.KindBeginObject); err != nil {
		return nil, err
	}

	var seen uint
	for dec.PeekKind() != jsontext.KindEndObject {
		key, err := dec.ReadValue()
		if err != nil {
			return nil, err
		}
		switch string(key) {
		case `"packs"`:
			if err := markField(&seen, indexSeenPacks, "packs"); err != nil {
				return nil, err
			}
			if err := decodePacksJSON(dec, idx); err != nil {
				return nil, err
			}
		default:
			// ignore unknown keys (e.g. historical "supersedes")
			if err := dec.SkipValue(); err != nil {
				return nil, err
			}
		}
	}

	if err := assertJSONKind(dec, jsontext.KindEndObject); err != nil {
		return nil, err
	}

	tok, err := dec.ReadToken()
	switch {
	case errors.Is(err, io.EOF):
	case err != nil:
		return nil, err
	default:
		return nil, errors.Errorf("trailing data after index JSON: %v", tok.Kind())
	}

	return idx, nil
}

func assertJSONKind(dec *jsontext.Decoder, want jsontext.Kind) error {
	tok, err := dec.ReadToken()
	if err != nil {
		return err
	}
	if tok.Kind() != want {
		return errors.Errorf("expected %v, got %v", want, tok.Kind())
	}
	return nil
}

func decodePacksJSON(dec *jsontext.Decoder, idx *Index) error {
	if err := assertJSONKind(dec, jsontext.KindBeginArray); err != nil {
		return err
	}

	for dec.PeekKind() != jsontext.KindEndArray {
		if err := decodePackJSON(dec, idx); err != nil {
			return err
		}
	}

	return assertJSONKind(dec, jsontext.KindEndArray)
}

func decodePackJSON(dec *jsontext.Decoder, idx *Index) error {
	if err := assertJSONKind(dec, jsontext.KindBeginObject); err != nil {
		return err
	}

	var (
		packIndex uint32
		seen      uint
		pending   []pack.Blob
	)

	for dec.PeekKind() != jsontext.KindEndObject {
		key, err := dec.ReadValue()
		if err != nil {
			return err
		}
		switch string(key) {
		case `"id"`:
			if err := markField(&seen, packSeenID, "id"); err != nil {
				return err
			}
			v, err := dec.ReadValue()
			if err != nil {
				return err
			}
			var packID restic.ID
			if err := packID.UnmarshalJSON(v); err != nil {
				return err
			}
			packIndex = idx.addToPacks(packID)
			for _, blob := range pending {
				idx.store(packIndex, blob)
			}
			pending = nil
		case `"blobs"`:
			if err := markField(&seen, packSeenBlobs, "blobs"); err != nil {
				return err
			}
			if err := assertJSONKind(dec, jsontext.KindBeginArray); err != nil {
				return err
			}
			for dec.PeekKind() != jsontext.KindEndArray {
				blob, err := decodeBlobJSON(dec)
				if err != nil {
					return err
				}
				if seen&packSeenID != 0 {
					idx.store(packIndex, blob)
				} else {
					pending = append(pending, blob)
				}
			}
			if err := assertJSONKind(dec, jsontext.KindEndArray); err != nil {
				return err
			}
		default:
			if err := dec.SkipValue(); err != nil {
				return err
			}
		}
	}

	if err := assertJSONKind(dec, jsontext.KindEndObject); err != nil {
		return err
	}

	if seen&packSeenID == 0 {
		return errors.Errorf("pack without ID found")
	}

	return nil
}

func decodeBlobJSON(dec *jsontext.Decoder) (pack.Blob, error) {
	if err := assertJSONKind(dec, jsontext.KindBeginObject); err != nil {
		return pack.Blob{}, err
	}

	var (
		blob pack.Blob
		seen uint
	)
	for dec.PeekKind() != jsontext.KindEndObject {
		key, err := dec.ReadValue()
		if err != nil {
			return pack.Blob{}, err
		}
		switch string(key) {
		case `"id"`:
			if err := markField(&seen, blobSeenID, "id"); err != nil {
				return pack.Blob{}, err
			}
			v, err := dec.ReadValue()
			if err != nil {
				return pack.Blob{}, err
			}
			if err := blob.ID.UnmarshalJSON(v); err != nil {
				return pack.Blob{}, err
			}
		case `"type"`:
			if err := markField(&seen, blobSeenType, "type"); err != nil {
				return pack.Blob{}, err
			}
			v, err := dec.ReadValue()
			if err != nil {
				return pack.Blob{}, err
			}
			if err := blob.Type.UnmarshalJSON(v); err != nil {
				return pack.Blob{}, err
			}
		case `"offset"`:
			if err := markField(&seen, blobSeenOffset, "offset"); err != nil {
				return pack.Blob{}, err
			}
			blob.Offset, err = decodeJSONUint(dec)
			if err != nil {
				return pack.Blob{}, err
			}
		case `"length"`:
			if err := markField(&seen, blobSeenLength, "length"); err != nil {
				return pack.Blob{}, err
			}
			blob.Length, err = decodeJSONUint(dec)
			if err != nil {
				return pack.Blob{}, err
			}
		case `"uncompressed_length"`:
			if err := markField(&seen, blobSeenUncompressedLength, "uncompressed_length"); err != nil {
				return pack.Blob{}, err
			}
			blob.UncompressedLength, err = decodeJSONUint(dec)
			if err != nil {
				return pack.Blob{}, err
			}
		default:
			if err := dec.SkipValue(); err != nil {
				return pack.Blob{}, err
			}
		}
	}

	if err := assertJSONKind(dec, jsontext.KindEndObject); err != nil {
		return pack.Blob{}, err
	}
	return blob, nil
}

// markField records that name has been seen. A second occurrence is an error.
func markField(seen *uint, bit uint, name string) error {
	if *seen&bit != 0 {
		return errors.Errorf("duplicate field %q", name)
	}
	*seen |= bit
	return nil
}

func decodeJSONUint(dec *jsontext.Decoder) (uint, error) {
	tok, err := dec.ReadToken()
	if err != nil {
		return 0, err
	}
	if tok.Kind() != jsontext.KindNumber {
		return 0, errors.Errorf("expected number, got %v", tok.Kind())
	}
	n, err := tok.Uint()
	if err != nil {
		return 0, err
	}
	if n > math.MaxUint32 {
		return 0, errors.Errorf("invalid unsigned integer %d", n)
	}
	return uint(n), nil
}
