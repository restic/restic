// Package json replaces encoding/json with jsoniter.
package json

import (
	"io"

	jsoniter "github.com/json-iterator/go"
)

type Decoder = jsoniter.Decoder

func NewDecoder(r io.Reader) *Decoder {
	return jsoniter.ConfigCompatibleWithStandardLibrary.NewDecoder(r)
}

type Encoder = jsoniter.Encoder

func NewEncoder(w io.Writer) *Encoder {
	return jsoniter.ConfigCompatibleWithStandardLibrary.NewEncoder(w)
}

func Marshal(v interface{}) ([]byte, error) {
	return jsoniter.ConfigCompatibleWithStandardLibrary.Marshal(v)
}

func MarshalIndent(v interface{}, prefix, indent string) ([]byte, error) {
	return jsoniter.ConfigCompatibleWithStandardLibrary.MarshalIndent(v, prefix, indent)
}

func Unmarshal(data []byte, v interface{}) error {
	return jsoniter.ConfigCompatibleWithStandardLibrary.Unmarshal(data, v)
}
