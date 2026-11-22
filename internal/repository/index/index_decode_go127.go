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

// decodeIndexJSON directly streams the index JSON into an Index without intermediate objects.
// Hand-rolled decoder for packJSON to significantly improve performance.
func decodeIndexJSON(buf []byte) (*Index, error) {
	dec := jsontext.NewDecoder(bytes.NewBuffer(buf))
	idx := NewIndex()

	if err := assertJSONKind(dec, jsontext.KindBeginObject); err != nil {
		return nil, err
	}

	for dec.PeekKind() != jsontext.KindEndObject {
		key, err := dec.ReadValue()
		if err != nil {
			return nil, err
		}
		switch string(key) {
		case `"packs"`:
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
		haveID    bool
		pending   []pack.Blob
	)

	for dec.PeekKind() != jsontext.KindEndObject {
		key, err := dec.ReadValue()
		if err != nil {
			return err
		}
		switch string(key) {
		case `"id"`:
			v, err := dec.ReadValue()
			if err != nil {
				return err
			}
			var packID restic.ID
			if err := packID.UnmarshalJSON(v); err != nil {
				return err
			}
			packIndex = idx.addToPacks(packID)
			haveID = true
			for _, blob := range pending {
				idx.store(packIndex, blob)
			}
			pending = nil
		case `"blobs"`:
			if err := assertJSONKind(dec, jsontext.KindBeginArray); err != nil {
				return err
			}
			for dec.PeekKind() != jsontext.KindEndArray {
				blob, err := decodeBlobJSON(dec)
				if err != nil {
					return err
				}
				if haveID {
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

	if !haveID {
		return errors.Errorf("pack without ID found")
	}

	return nil
}

func decodeBlobJSON(dec *jsontext.Decoder) (pack.Blob, error) {
	if err := assertJSONKind(dec, jsontext.KindBeginObject); err != nil {
		return pack.Blob{}, err
	}

	var blob pack.Blob
	for dec.PeekKind() != jsontext.KindEndObject {
		key, err := dec.ReadValue()
		if err != nil {
			return pack.Blob{}, err
		}
		switch string(key) {
		case `"id"`:
			v, err := dec.ReadValue()
			if err != nil {
				return pack.Blob{}, err
			}
			if err := blob.ID.UnmarshalJSON(v); err != nil {
				return pack.Blob{}, err
			}
		case `"type"`:
			v, err := dec.ReadValue()
			if err != nil {
				return pack.Blob{}, err
			}
			if err := blob.Type.UnmarshalJSON(v); err != nil {
				return pack.Blob{}, err
			}
		case `"offset"`:
			blob.Offset, err = decodeJSONUint(dec)
			if err != nil {
				return pack.Blob{}, err
			}
		case `"length"`:
			blob.Length, err = decodeJSONUint(dec)
			if err != nil {
				return pack.Blob{}, err
			}
		case `"uncompressed_length"`:
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
